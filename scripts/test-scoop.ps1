# Integration test only: install a snapshot with Scoop on an ephemeral Windows
# runner. The generated manifest's only semantic change is its x64 download URL.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$SnapshotDir,
    [Parameter(Mandatory)][string]$EvidenceDir,
    [switch]$ValidateOnly
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$snapshot = (Resolve-Path -LiteralPath $SnapshotDir).Path
$evidence = (New-Item -ItemType Directory -Force -Path $EvidenceDir).FullName
$manifestPath = Join-Path $snapshot 'scoop/envguard.json'
$manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json -AsHashtable
$original = $manifest | ConvertTo-Json -Depth 50 -Compress
$version = (Get-Content -Raw (Join-Path $snapshot 'metadata.json') | ConvertFrom-Json).version
if ($manifest.version -ne $version) { throw 'Manifest and snapshot versions differ' }
$x64 = $manifest.architecture.'64bit'
$archiveName = [IO.Path]::GetFileName(([uri]$x64.url).AbsolutePath)
if ($archiveName -ne "envguard_${version}_windows_amd64.zip") { throw 'Unexpected x64 archive name' }
if ((ConvertTo-Json -InputObject $x64.bin -Compress) -ne '["envguard.exe"]') { throw 'Unexpected Scoop binary entry' }
$archivePath = Join-Path $snapshot $archiveName
$actualHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $archivePath).Hash.ToLowerInvariant()
if ($actualHash -ne $x64.hash) { throw 'Snapshot archive does not match the generated manifest hash' }
$checksumLines = @(Get-Content (Join-Path $snapshot 'checksums.txt') | Where-Object {
    $_ -match ('^[a-fA-F0-9]{64}\s+\*?' + [regex]::Escape($archiveName) + '$')
})
if ($checksumLines.Count -ne 1 -or ($checksumLines[0] -split '\s+')[0] -ne $actualHash) {
    throw 'Snapshot checksum entry is missing or mismatched'
}
Copy-Item -LiteralPath $manifestPath -Destination (Join-Path $evidence 'original-manifest.json')

$listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
$listener.Start()
$port = $listener.LocalEndpoint.Port
$listener.Stop()
$originalURL = $x64.url
$x64.url = "http://127.0.0.1:$port/$archiveName"
$localManifest = Join-Path $evidence 'envguard.json'
$manifest | ConvertTo-Json -Depth 50 | Set-Content -Encoding utf8 $localManifest
# Prove that version, hashes, architecture selection and every other field stayed intact.
$restored = Get-Content -Raw $localManifest | ConvertFrom-Json -AsHashtable
$restored.architecture.'64bit'.url = $originalURL
if (($restored | ConvertTo-Json -Depth 50 -Compress) -ne $original) { throw 'Manifest changed beyond its mirror URL' }
$result = [ordered]@{ version = $version; archive_sha256 = $actualHash; url_only_change = $true; platform = 'Windows x64'; installed = $false; uninstalled = $false }
if ($ValidateOnly) {
    $result | ConvertTo-Json | Set-Content (Join-Path $evidence 'validation.json')
    return
}
if (-not $IsWindows -or [Runtime.InteropServices.RuntimeInformation]::OSArchitecture -ne 'X64') {
    throw 'This runtime test requires Windows x64; use -ValidateOnly for artifact checks elsewhere'
}
if (-not $env:RUNNER_TEMP) { throw 'RUNNER_TEMP is required for the isolated installation' }
$root = Join-Path $env:RUNNER_TEMP 'envguard-scoop-test'
if (Test-Path -LiteralPath $root) { throw 'Refusing to reuse an existing Scoop test installation' }
$env:SCOOP = Join-Path $root 'scoop'
$env:SCOOP_GLOBAL = Join-Path $root 'global'
$env:XDG_CONFIG_HOME = Join-Path $root 'config'
$cache = Join-Path $root 'cache'
$installer = Join-Path $evidence 'install-scoop.ps1'
# Official Scoop installer, pinned to the exact source reviewed for this test.
$installerCommit = '1e2f334083d609986d8c8bc9e31ae8e87c39fab4'
$server = $null
$scoop = Join-Path $env:SCOOP 'shims/scoop.ps1'
try {
    Invoke-WebRequest "https://raw.githubusercontent.com/ScoopInstaller/Install/$installerCommit/install.ps1" -OutFile $installer
    $result.installer_commit = $installerCommit
    $result.installer_sha256 = (Get-FileHash -Algorithm SHA256 $installer).Hash.ToLowerInvariant()
    $admin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    $global:LASTEXITCODE = 0
    & $installer -ScoopDir $env:SCOOP -ScoopGlobalDir $env:SCOOP_GLOBAL -ScoopCacheDir $cache -RunAsAdmin:$admin *>&1 |
        Tee-Object -FilePath (Join-Path $evidence 'bootstrap.log')
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path $scoop)) { throw 'Official Scoop bootstrap failed' }
    $python = (Get-Command python -ErrorAction Stop).Source
    $server = Start-Process $python -ArgumentList @('-m', 'http.server', "$port", '--bind', '127.0.0.1') -WorkingDirectory $snapshot -PassThru -RedirectStandardOutput (Join-Path $evidence 'mirror.stdout') -RedirectStandardError (Join-Path $evidence 'mirror.stderr')
    $ready = $false
    for ($attempt = 0; $attempt -lt 20; $attempt++) {
        try {
            Invoke-WebRequest "http://127.0.0.1:$port/checksums.txt" -Method Head -TimeoutSec 1 | Out-Null
            $ready = $true
            break
        } catch { Start-Sleep -Milliseconds 250 }
    }
    if (-not $ready) { throw 'Local snapshot mirror did not start' }
    & $scoop install $localManifest *>&1 | Tee-Object -FilePath (Join-Path $evidence 'install.log')
    if ($LASTEXITCODE -ne 0) { throw 'Scoop package installation failed' }
    $result.installed = $true
    $shim = Join-Path $env:SCOOP 'shims/envguard.exe'
    if (-not (Test-Path $shim)) { throw 'Scoop did not create the executable shim' }
    $versionOutput = & $shim version
    $versionExit = $LASTEXITCODE
    $versionOutput | Set-Content (Join-Path $evidence 'version.stdout')
    if ($versionExit -ne 0 -or $versionOutput[0] -ne "envguard $version") { throw 'Installed shim version does not match the snapshot' }
    $demo = Join-Path $PSScriptRoot '../examples/demo-app'
    $checkOutput = & $shim --root $demo check --format json 2> (Join-Path $evidence 'check.stderr')
    $checkExit = $LASTEXITCODE
    $checkOutput | Set-Content (Join-Path $evidence 'check.json')
    $report = ($checkOutput -join "`n") | ConvertFrom-Json
    if ($checkExit -ne 1 -or $report.schema_version -ne 1 -or @($report.findings).Count -eq 0) {
        throw 'Installed shim did not report the demo findings with exit code 1'
    }
    $result.version_exit = $versionExit
    $result.check_exit = $checkExit
    $result.findings = @($report.findings).Count
} finally {
    if ($result.installed) {
        & $scoop uninstall envguard *>&1 | Tee-Object -FilePath (Join-Path $evidence 'uninstall.log')
        $result.uninstalled = $LASTEXITCODE -eq 0 -and -not (Test-Path (Join-Path $env:SCOOP 'shims/envguard.exe'))
    }
    if ($null -ne $server -and -not $server.HasExited) { Stop-Process -Id $server.Id -Force }
    $result | ConvertTo-Json | Set-Content (Join-Path $evidence 'result.json')
}
if (-not $result.uninstalled) { throw 'Scoop package uninstall failed' }

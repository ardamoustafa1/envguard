# Installing EnvGuard

EnvGuard is a single static binary for Linux, macOS and Windows (amd64 and
arm64). Every method below installs the same binary. None of them needs
network access when EnvGuard *runs*: EnvGuard itself never makes network
calls. Only the installation downloads something.

| Method | Command |
|---|---|
| [Homebrew](#homebrew-macos-linux) | `brew install ardamoustafa1/tap/envguard` |
| [npm / npx](#npm-npx) | `npx envguard-cli check` |
| [install.sh](#installsh) | `curl -fsSL https://raw.githubusercontent.com/ardamoustafa1/envguard/main/install.sh \| sh` |
| [Go](#go-install) | `go install github.com/ardamoustafa1/envguard/cmd/envguard@latest` |
| [Docker](#docker) | `docker run --rm -v "$PWD:/repo" ghcr.io/ardamoustafa1/envguard check` |
| [Scoop](#scoop-windows) | `scoop install envguard` (after adding the bucket) |
| [deb / rpm / apk](#linux-packages) | from the release page |
| [GitHub Action](#github-actions) | `uses: ardamoustafa1/envguard@v1` |
| [pre-commit](#pre-commit), [lefthook](#lefthook), [husky](#husky) | Git hooks |
| [Manual download](#manual-download) | from the [releases page](https://github.com/ardamoustafa1/envguard/releases) |

Check the installation with `envguard version`.

## Homebrew (macOS, Linux)

```bash
brew install ardamoustafa1/tap/envguard
brew upgrade envguard
```

The tap is a Homebrew *cask* (GoReleaser's current way of publishing prebuilt
binaries). It installs the release archive for your platform, verified by its
SHA-256 checksum.

## npm / npx

```bash
npx envguard-cli doctor                 # run once, no install
npm install --save-dev envguard-cli     # per project; then `envguard check` in npm scripts
npm install --global envguard-cli
```

The npm package is called **`envguard-cli`**, not `envguard`: the `@envguard`
scope belongs to someone else, and npm refuses new names that differ from an
existing package (`env-guard`) only by punctuation. The installed command is
still `envguard`.

How it works (the same pattern esbuild uses): `envguard-cli` contains only a
small JavaScript launcher. The native binary comes from one of
`envguard-cli-{darwin,linux,win32}-{x64,arm64}`, which are
`optionalDependencies` with `os`/`cpu` fields, so npm downloads only the one
for your machine. There is **no postinstall script** and nothing is
downloaded at install time beyond the npm packages themselves.

- An unsupported platform gets a clear error that lists the supported ones.
- If the platform package is missing (installed with `--omit=optional`, or a
  lockfile created on another OS), the launcher says which package to add.
- `ENVGUARD_BINARY=/path/to/envguard` makes the launcher run another binary.
- Packages are published with [npm provenance](https://docs.npmjs.com/generating-provenance-statements),
  linking each version to the GitHub Actions run that built it.

## install.sh

```bash
curl -fsSL https://raw.githubusercontent.com/ardamoustafa1/envguard/main/install.sh | sh
# a specific version:
curl -fsSL https://raw.githubusercontent.com/ardamoustafa1/envguard/main/install.sh | sh -s -- 1.2.3
```

The script (POSIX `sh`; checked with shellcheck in CI) detects the OS and
architecture (including Apple silicon under Rosetta, and Git Bash on
Windows), downloads the release archive and `checksums.txt`, **refuses to
install if the SHA-256 checksum does not match**, and installs to
`/usr/local/bin` when run as root, else `~/.local/bin`. It needs `curl` or
`wget`, `tar` (`unzip` on Windows), and `sha256sum`, `shasum` or `openssl`.

| Variable | Effect |
|---|---|
| `ENVGUARD_INSTALL_DIR` | Install somewhere else. |
| `ENVGUARD_RELEASES_URL` | Download from a mirror with the same layout (`<url>/download/v1.2.3/…`, `<url>/latest`). |
| `ENVGUARD_VERIFY_SIGNATURE=1` | Also verify the Sigstore signature of `checksums.txt` (needs `cosign`). |

Prefer to read it first? `curl -fsSLO …/install.sh && less install.sh && sh install.sh`.

## go install

```bash
go install github.com/ardamoustafa1/envguard/cmd/envguard@latest
go install github.com/ardamoustafa1/envguard/cmd/envguard@v1.2.3
```

Needs Go 1.26 or newer. `envguard version` shows the module version (and the
commit and date when built from a git checkout), read from the Go build info
because `go install` does not set release ldflags.

## Docker

```bash
docker run --rm -v "$PWD:/repo" ghcr.io/ardamoustafa1/envguard check
docker run --rm -v "$PWD:/repo" ghcr.io/ardamoustafa1/envguard doctor
docker run --rm -v "$PWD:/repo" ghcr.io/ardamoustafa1/envguard:1.2.3 check --env production
```

- Multi-arch (`linux/amd64`, `linux/arm64`) image based on
  `gcr.io/distroless/static-debian12:nonroot`: about 3 MB, no shell, runs as
  the non-root user `nonroot` (uid 65532) in `WORKDIR /repo`. The default
  command is `check`.
- Tags: `1.2.3`, `1.2`, `1`, `latest`. Images are signed with cosign (keyless).
- The image has no `git`, so the git-based checks (EG008, EG009) are skipped.
- On Linux, uid 65532 must be able to read your files. If they are not
  world-readable, run as yourself: `docker run --rm --user "$(id -u):$(id -g)" -v "$PWD:/repo" …`.

## Scoop (Windows)

```powershell
scoop bucket add envguard https://github.com/ardamoustafa1/scoop-bucket
scoop install envguard
```

## Linux packages

Each release has `.deb`, `.rpm` and `.apk` packages for amd64 and arm64
(binary in `/usr/bin/envguard`):

```bash
sudo dpkg -i envguard_1.2.3_linux_amd64.deb      # Debian, Ubuntu
sudo rpm -i envguard_1.2.3_linux_amd64.rpm       # Fedora, RHEL
sudo apk add --allow-untrusted envguard_1.2.3_linux_amd64.apk   # Alpine
```

## GitHub Actions

```yaml
jobs:
  envguard:
    runs-on: ubuntu-latest        # also macos-latest, windows-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v5
      - uses: ardamoustafa1/envguard@v1
```

The action downloads the release binary for the runner, verifies its SHA-256
checksum, caches it, and runs `envguard check`. Findings appear as pull
request annotations and in the job summary; the step fails on errors.

| Input | Default | Meaning |
|---|---|---|
| `version` | `latest` | `1.2.3`, `v1.2.3` or `latest`. Pin it for reproducible builds. |
| `args` | `check` | Arguments, e.g. `check --env production` or `doctor`. Split on whitespace, never evaluated by a shell. |
| `working-directory` | `.` | Directory to analyze (`--root`). |
| `fail-on` | `error` | `error`, `warning`, `info` or `never`, added to `check`/`doctor` unless `args` has `--fail-on`. |
| `sarif` | `false` | Also upload a SARIF report to code scanning (see below). |
| `sarif-file` | `envguard.sarif` | Where the SARIF report is written. |

Outputs: `version`, `sarif-file`.

**Code scanning.** With `sarif: true` the action writes a SARIF report and
uploads it with `github/codeql-action/upload-sarif`. That needs code scanning
enabled for the repository and this permission on the job:

```yaml
    permissions:
      contents: read
      security-events: write
    steps:
      - uses: actions/checkout@v5
      - uses: ardamoustafa1/envguard@v1
        with:
          sarif: true
```

Pull requests from forks get a read-only token, so the upload fails there;
run it on `push` to the default branch, or skip `sarif` for fork PRs.

Without the action: any install method plus `envguard check` works too, since
EnvGuard switches to annotations by itself when `GITHUB_ACTIONS=true`.

## pre-commit

```yaml
# .pre-commit-config.yaml
repos:
  - repo: https://github.com/ardamoustafa1/envguard
    rev: v1.2.3
    hooks:
      - id: envguard
        # args: [--env, production]
```

The hook builds EnvGuard from source with Go (`language: golang`; pre-commit
installs Go if it is missing) and runs `envguard check` on every commit
(`pass_filenames: false`; the whole repository is checked).

## lefthook

```yaml
# lefthook.yml
pre-commit:
  commands:
    envguard:
      run: envguard check
```

`envguard` must be on `PATH` (any method above). In a Node project, use
`run: npx envguard check` with `envguard-cli` as a dev dependency.

## husky

```bash
npm install --save-dev husky envguard-cli
npx husky init
echo "npx envguard check" > .husky/pre-commit
```

## Manual download

Download the archive for your platform from the
[releases page](https://github.com/ardamoustafa1/envguard/releases)
(`envguard_<version>_<os>_<arch>.tar.gz`, `.zip` on Windows), verify it (below),
extract it and put `envguard` on your `PATH`. Each archive contains the
binary, `LICENSE` and `README.md`; an SPDX SBOM (`*.sbom.json`) sits next to it.

## Verifying downloads

Every release has `checksums.txt` (SHA-256 of every archive and package) and
`checksums.txt.sigstore.json`, a keyless [Sigstore](https://www.sigstore.dev)
signature made by the release workflow. No keys to manage: the certificate
proves the file was signed by this repository's `release.yml` on a `v*` tag.

```bash
# 1. The checksums file is authentic:
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/ardamoustafa1/envguard/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

# 2. Your download matches it:
sha256sum --ignore-missing -c checksums.txt     # macOS: shasum -a 256 --ignore-missing -c checksums.txt
```

Docker images:

```bash
cosign verify ghcr.io/ardamoustafa1/envguard:1.2.3 \
  --certificate-identity-regexp '^https://github.com/ardamoustafa1/envguard/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

npm packages: `npm audit signatures` checks registry signatures and provenance.

## Building from source

```bash
git clone https://github.com/ardamoustafa1/envguard && cd envguard
make build          # bin/envguard with version information
make snapshot       # the full release locally in dist/ (nothing is published)
```

`make snapshot` runs GoReleaser in snapshot mode (archives, checksums, SBOMs
if `syft` is installed, deb/rpm/apk, Homebrew cask and Scoop manifest files,
Docker images if Docker is running) and then builds and smoke-tests the npm
packages in `dist/npm`. Signing runs only in CI.

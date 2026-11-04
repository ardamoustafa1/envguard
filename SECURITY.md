# Security policy

## Supported versions

Only the latest minor release gets security fixes. Upgrade before reporting
if you are on an older one.

## Report a vulnerability

Report privately with GitHub private vulnerability reporting:
<https://github.com/ardamoustafa1/envguard/security/advisories/new>

Do not open a public issue for a vulnerability. Do not paste real secrets in
a report. A minimal repository with fake values is enough.

## What is in scope

- A secret value reaching any output: terminal, JSON, SARIF, GitHub
  annotations, job summary, error messages, verbose logs.
- Any network access, from EnvGuard or from a dependency.
- Writing files other than the documented exceptions (`envguard init`,
  `--output`, `$GITHUB_STEP_SUMMARY`).
- Path traversal or symlink escapes out of `--root`.
- Command execution through git configuration or any other input.
- Terminal injection: a file name, directory name or YAML key in an analyzed
  repository that reaches text output as an escape sequence or a hidden
  bidirectional character.
- A crash (panic, goroutine dump) on crafted input.
- Denial of service from crafted input: quadratic or unbounded time or
  memory in the parsers, the scanner, the YAML handling or the output.

False positives and false negatives of rules are not vulnerabilities. Use the
false positive issue template for those.

## What to expect

- Acknowledgement within 3 working days.
- First assessment within 7 days.
- A fix or a mitigation plan within 30 days for confirmed issues, sooner for
  secret leaks.
- Credit in the advisory and the changelog, unless you prefer not to be named.

## How EnvGuard protects values

EnvGuard is offline and read-only, and it never prints a secret value. Value
text reaches output only through `internal/mask`. These tests enforce it:

- `TestLeakCanary` (`cmd/envguard/leak_canary_test.go`) runs every command and
  format on a repository where every value contains `CNRY`. No output may
  contain it.
- `TestOffline` (`cmd/envguard/offline_test.go`) fails if any network client
  package is in the dependency graph.
- `TestInitNeverCopiesValues` (`cmd/envguard/init_test.go`) checks that
  `envguard init` never writes a value.
- `TestInspectIgnoresMaliciousFsmonitor` guards the git calls against
  repository configuration that would run programs, and `TestInspectUnusualNames`
  checks that unusual file names (spaces, quotes, leading dashes, newlines) are
  never interpreted as options or commands.
- `TestNoNetworkOrTelemetryImports` parses the imports of every production
  file and fails on network or telemetry packages.
- `TestCommandsDoNotModifyTheRepository` and `TestOnlyDocumentedFilesWriteToDisk`
  enforce read-only behavior.
- `TestHostileRepositoryNeverCrashesOrInjects` runs everything on a repository
  of huge, binary, deeply nested, invalid-UTF-8 and symlinked files with file
  names that carry terminal escape sequences.

`make security` runs gosec and govulncheck; every remaining gosec finding is
justified in a `#nosec` comment next to the code, and the parsers, scanner,
Compose and workflow readers and config loader are fuzzed (`make fuzz`).

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full list of principles.

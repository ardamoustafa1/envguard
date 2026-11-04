# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-11-04

First release.

### Added

- Commands: `check`, `doctor`, `diff`, `explain`, `init` and `version`.
  Running `envguard` without a command runs `doctor`.
- 16 rules, EG001 to EG016, each documented in [docs/rules](docs/rules/README.md)
  and available through `envguard explain`.
- Output formats: `pretty`, `doctor`, `json`, `sarif` (2.1.0) and `github`
  (workflow annotations plus a job summary). `--output FILE` writes a report
  to a file.
- `check --process-env` validates the process environment against templates
  and schema, for use in container entrypoints.
- Source scanning for JavaScript, TypeScript, Vue, Svelte, Python, Go, Ruby,
  PHP, Rust, Java, Kotlin, C#, Elixir and shell.
- Docker Compose and GitHub Actions workflow parsing.
- Optional `.envguard.yaml` schema with types, constraints, production rules
  and a JSON Schema in `docs/envguard.schema.json`.
- Secret detection for templates and tracked files. Values are masked in all
  output.
- Distribution: GoReleaser archives for Linux, macOS and Windows (amd64 and
  arm64) with signed checksums and SBOMs, the `envguard-cli` npm package,
  a multi-arch Docker image, a Homebrew cask, a Scoop manifest, deb, rpm and
  apk packages, `install.sh`, a GitHub Action and a pre-commit hook.
- Leak canary, offline and determinism tests that enforce the core
  principles.

### Security

- Offline and read-only by design, enforced by tests: no network or telemetry
  imports, no file writes outside `init`, `--output` and the GitHub job
  summary, and secret values reach output only through a single masking
  package.
- Safe on untrusted repositories: file and directory names are shown with
  terminal control characters escaped, symlinks leaving the root are never
  followed, `env_file` paths outside the root are ignored, panics in rules and
  scanner workers never crash the run or print a value, and git runs without a
  shell and with the repository's `core.fsmonitor` disabled.
- The dotenv parser, source scanner, Compose and workflow readers and the
  config loader are fuzzed. `make security` runs gosec and govulncheck.

### Known limitations

See [docs/how-it-works.md](docs/how-it-works.md#known-limitations): dynamic
keys are not analyzed, reads in Rails ERB/YAML files and by frameworks
themselves are not seen (EG004 can report such variables as unused), Go
`envPrefix` names are inferred, and git history is not scanned (use gitleaks).

[Unreleased]: https://github.com/ardamoustafa1/envguard/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/ardamoustafa1/envguard/releases/tag/v0.1.0

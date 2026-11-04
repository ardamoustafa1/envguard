# Roadmap

Plans, not promises. Items move when someone does the work. No dates.
Every item must respect the [principles](../CONTRIBUTING.md#principles):
offline, read-only, never print values, deterministic, low false positives.

Small tasks to start with are in [good-first-issues.md](roadmap/good-first-issues.md).

## v0.2: more sources

- **GitLab CI parsing.** `.gitlab-ci.yml` has `variables:` and `$VAR`
  references, the same cross-check EnvGuard already does for GitHub Actions.
  Many teams use it, and the workflow parser shows how to do it.
- **Kubernetes manifests and Helm values.** `env:`, `envFrom`, `secretKeyRef`
  and `configMapKeyRef` declare variables that code reads. Helm values are
  parsed as YAML only. Charts are never rendered, because rendering needs
  templating logic and could run untrusted input.
- **Terraform variables.** `variable` blocks and `TF_VAR_*` names are another
  place where config drifts from code.

## v0.3: help fixing

- **`envguard fix`.** Apply the safe, mechanical fixes: add a missing key to
  an env file, remove a duplicate, add a `.gitignore` entry. It asks for
  explicit interactive confirmation for every change and has a `--dry-run`.
  It would be the **second exception to the read-only principle**, after
  `envguard init`, so it needs its own tests. It must never write a value:
  added keys are empty. It must never be the default and never run in CI
  without an explicit flag.
- **VS Code extension.** Show findings inline, using
  `envguard check --format json` ([schema](json-output.md)). The extension
  shells out to the binary, so it adds no logic and no network code.

## v1.0: stability

The first stable release freezes what users and tools depend on:

- JSON output: `schema_version` stays at 1. Fields are only added, never
  removed or renamed.
- SARIF: a stable 2.1.0 shape and stable `partialFingerprints`.
- Rule IDs: permanent, never reused. Rule names and default severities change
  only in a major release.
- Exit codes: 0, 1 and 2 keep their meaning.
- Configuration: `version: 1` of `.envguard.yaml` keeps working.
- Deprecation policy: a deprecated flag, rule or key warns for at least one
  minor release before removal in the next major release, and the changelog
  says so.

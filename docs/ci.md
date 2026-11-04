# Using EnvGuard in CI

EnvGuard is a single offline binary. In CI you install it, run `envguard check`
and let the exit code decide. See [installation](installation.md) for every
install method.

## Exit codes and --fail-on

| Code | Meaning |
|---|---|
| 0 | No findings at or above `--fail-on`. |
| 1 | At least one finding at or above `--fail-on`. |
| 2 | Configuration or runtime error (bad flag, invalid `.envguard.yaml`, unreadable root, or an `--env`, `--app` or `--format` value that does not exist). |

`--fail-on` takes `error` (default for `check`), `warning`, `info` or `never`.
You can also set `fail_on` in [`.envguard.yaml`](configuration.md#fail_on); the
flag wins.

A typo in `--env` (for example `--env prodction`) is exit code 2, not a green
run: EnvGuard refuses to report "no problems" for an environment it never
found. Aliases such as `prod` and `dev` are accepted.

A rollout that works well:

1. Start with the default, `--fail-on error`. This blocks only real breakage:
   missing variables, committed env files, secrets in templates.
2. Fix or [suppress](configuration.md#ignore) what is left at `warning`.
3. Switch to `--fail-on warning`. Keep `info` findings as advice.

Use `--fail-on never` when you only want reports (for example SARIF upload) and
a separate step that decides about failure.

## Output formats

`check` and `doctor` accept `--format pretty|doctor|json|sarif|github` and
`--output FILE`. `--output` writes the report to a file (creating parent
directories, never with colors) instead of stdout.

```sh
envguard check --format json  --output reports/envguard.json
envguard check --format sarif --output reports/envguard.sarif
```

The JSON format is documented in [JSON output](json-output.md). Findings never
contain secret values, so reports are safe to keep as CI artifacts.

When `GITHUB_ACTIONS=true` and you do not pass `--format`, `check` uses the
`github` format on its own: findings become annotations on the pull request,
and a Markdown job summary is appended to `$GITHUB_STEP_SUMMARY` when that
variable is set.

## GitHub Actions

### The action

```yaml
name: EnvGuard
on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read

jobs:
  envguard:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: ardamoustafa1/envguard@v1
```

The action downloads the release binary, verifies its SHA-256 checksum, caches
it and runs `envguard --root <working-directory> <args>`.

| Input | Default | Meaning |
|---|---|---|
| `version` | `latest` | Version to install: `1.2.3`, `v1.2.3` or `latest`. |
| `args` | `check` | Arguments for envguard, split on whitespace (never evaluated), for example `check --env production` or `doctor`. |
| `working-directory` | `.` | Directory to analyze, passed as `--root`. |
| `fail-on` | `error` | Threshold appended to `check` and `doctor`. An explicit `--fail-on` in `args` wins. Empty keeps the command default. |
| `sarif` | `false` | Also write a SARIF report and upload it to code scanning. |
| `sarif-file` | `envguard.sarif` | Where to write the SARIF report. |

Outputs: `version` (the installed version) and `sarif-file` (its path, when
`sarif` is true).

Pin `version` for reproducible builds:

```yaml
      - uses: ardamoustafa1/envguard@v1
        with:
          version: 1.2.3
          args: check --env production,staging
          fail-on: warning
          working-directory: apps/api
```

### Code scanning with SARIF

With `sarif: true` the action writes the report with `--fail-on never`, uploads
it with `github/codeql-action/upload-sarif` (category `envguard`), and then runs
the normal check, so the step still fails according to `fail-on`. The job needs
`security-events: write`, and code scanning must be enabled for the repository.

```yaml
permissions:
  contents: read
  security-events: write

jobs:
  envguard:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: ardamoustafa1/envguard@v1
        with:
          sarif: true
```

Findings then show up under Security, Code scanning, and as annotations in the
pull request diff.

**Fork pull requests.** For pull requests from forks, `GITHUB_TOKEN` is
read-only, so the SARIF upload fails with a permissions error. Either enable
SARIF only for non-fork runs:

```yaml
      - uses: ardamoustafa1/envguard@v1
        with:
          sarif: ${{ github.event.pull_request.head.repo.full_name == github.repository || github.event_name == 'push' }}
```

or skip SARIF on pull requests and rely on the annotations and job summary,
which do not need extra permissions.

### Without the action

```yaml
      - uses: actions/checkout@v4
      - name: Install EnvGuard
        run: curl -fsSL https://raw.githubusercontent.com/ardamoustafa1/envguard/main/install.sh | sh
      - name: Check
        run: envguard check
```

`install.sh` puts the binary in `/usr/local/bin` as root, otherwise in
`~/.local/bin`. Set `ENVGUARD_INSTALL_DIR` to choose, and add that directory to
`$GITHUB_PATH` if it is not on `PATH`.

## GitLab CI

The simplest setup installs the binary in a regular image:

```yaml
envguard:
  stage: test
  image: alpine:3.20
  before_script:
    - apk add --no-cache curl git
    - curl -fsSL https://raw.githubusercontent.com/ardamoustafa1/envguard/main/install.sh | ENVGUARD_INSTALL_DIR=/usr/local/bin sh
  script:
    - envguard check --format sarif --output envguard.sarif --fail-on never
    - envguard check
  artifacts:
    when: always
    paths:
      - envguard.sarif
```

`git` is optional, but without it the git-based checks (EG008, EG009 and the
tracked-file part of EG010) are skipped. Use `--format json` instead of `sarif`
if you want to process the report yourself.

The Docker image cannot be used directly as a GitLab job image. It is
distroless and has no shell, and GitLab runs `script:` lines through one. If
you build your own CI image, copy the binary into it instead:

```dockerfile
COPY --from=ghcr.io/ardamoustafa1/envguard:1.2.3 /usr/local/bin/envguard /usr/local/bin/envguard
```

## CircleCI

```yaml
version: 2.1

jobs:
  envguard:
    docker:
      - image: cimg/base:stable
    steps:
      - checkout
      - run:
          name: Install EnvGuard
          command: curl -fsSL https://raw.githubusercontent.com/ardamoustafa1/envguard/main/install.sh | ENVGUARD_INSTALL_DIR="$HOME/bin" sh
      - run:
          name: Check env configuration
          command: |
            export PATH="$HOME/bin:$PATH"
            envguard check --format json --output reports/envguard.json --fail-on never
            envguard check
      - store_artifacts:
          path: reports

workflows:
  main:
    jobs:
      - envguard
```

## Git hooks

Run EnvGuard before every commit with pre-commit, lefthook or husky. The setup
for each is in [installation](installation.md#pre-commit). A hook runs the same
`envguard check`, so the same `.envguard.yaml` applies.

## Docker

```sh
docker run --rm -v "$PWD:/repo" ghcr.io/ardamoustafa1/envguard check
docker run --rm -v "$PWD:/repo" ghcr.io/ardamoustafa1/envguard check --env production
```

The image runs as `nonroot` and its working directory is `/repo`. If the
mounted files are not readable by that user, run as yourself:

```sh
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD:/repo" ghcr.io/ardamoustafa1/envguard check
```

The container has no `git` binary, so the git-based checks (EG008, EG009, the
tracked-file part of EG010) are skipped. Run the binary on the host or in CI if
you need them.

### Validating the process environment at startup

`--process-env` checks the environment of the running process instead of env
files. It compares it against the templates and schema of one environment and
runs only EG001 (unset), EG005 (empty), EG014 (invalid) and EG015 (unsafe
production values). It reads no env files, Compose files or workflows.

Fail a container early when configuration is missing:

```dockerfile
FROM node:22-slim
COPY --from=ghcr.io/ardamoustafa1/envguard:1.2.3 /usr/local/bin/envguard /usr/local/bin/envguard
WORKDIR /app
COPY . .
# The templates (.env.example, .env.production.example) and .envguard.yaml
# must be in the image; real .env files are not needed.
ENTRYPOINT ["sh", "-c", "envguard check --process-env --env production --quiet && exec \"$@\"", "--"]
CMD ["node", "server.js"]
```

`--quiet` prints nothing and reports through the exit code only. Drop it to see
the findings in the container log. Values never appear in the output.

The same check works as a Kubernetes init container. The pod's environment
(from `env`, `envFrom`, ConfigMaps and Secrets) is validated before the app
container starts:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: api
spec:
  initContainers:
    - name: envguard
      image: ghcr.io/ardamoustafa1/envguard:1.2.3
      workingDir: /config
      args: ["check", "--process-env", "--env", "production"]
      envFrom:
        - secretRef:
            name: api-secrets
        - configMapRef:
            name: api-config
      volumeMounts:
        - name: schema
          mountPath: /config
  containers:
    - name: api
      image: registry.example.com/api:1.0.0
      envFrom:
        - secretRef:
            name: api-secrets
        - configMapRef:
            name: api-config
  volumes:
    - name: schema
      configMap:
        name: envguard-schema   # holds .env.example and .envguard.yaml
```

Choose one `--env` per run (more than one is an error) and, in a monorepo, one
`--app`.

<p align="center">
  <img src="docs/logo.svg" alt="EnvGuard" width="320">
</p>

<p align="center">
  <b>Your code knows which env vars it needs. EnvGuard makes sure your config agrees — before you deploy.</b>
</p>

<p align="center">
  <a href="https://github.com/ardamoustafa1/envguard/actions/workflows/ci.yml"><img src="https://github.com/ardamoustafa1/envguard/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/ardamoustafa1/envguard/releases/latest"><img src="https://img.shields.io/github/v/release/ardamoustafa1/envguard" alt="Release"></a>
  <a href="https://goreportcard.com/report/github.com/ardamoustafa1/envguard"><img src="https://goreportcard.com/badge/github.com/ardamoustafa1/envguard" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/ardamoustafa1/envguard" alt="License: MIT"></a>
  <a href="https://www.npmjs.com/package/envguard-cli"><img src="https://img.shields.io/npm/v/envguard-cli?label=npm" alt="npm"></a>
</p>

<p align="center">
  <img src="docs/demo.gif" alt="EnvGuard running doctor, check, explain and diff on a small demo project" width="900">
</p>

## Try it in 10 seconds

```bash
npx envguard-cli                                                    # no install
brew install ardamoustafa1/tap/envguard                          # macOS, Linux
go install github.com/ardamoustafa1/envguard/cmd/envguard@latest
docker run --rm -v "$PWD:/repo" ghcr.io/ardamoustafa1/envguard check
```

Run it in any repository. It needs no config, makes no network calls and
never changes your files. Plain `envguard` prints a health report;
`envguard check` is the short, CI-friendly version that exits `1` on errors.

> The npm package is called `envguard-cli` because the name `envguard` is not
> available. The command it installs is still `envguard`. More ways to
> install (Scoop, deb/rpm/apk, install script, checksums and signatures):
> [docs/installation.md](docs/installation.md).

## What it catches

EnvGuard reads your code, `.env` files, `.env.example` templates, Docker
Compose files and GitHub workflows, then checks that they agree. These
examples come from [examples/demo-app](examples/demo-app), a small Node.js +
Python project that is broken on purpose. Every output below is real, and a
test fails if it drifts from what EnvGuard prints.

### A variable is missing in production

`.env.production.example` says production needs `STRIPE_SECRET_KEY`. The real
`.env.production` does not set it. The app starts, then fails at the first payment.

<!-- envguard-file path=.env.production.example -->
```dotenv
CDN_URL=
STRIPE_SECRET_KEY=
```

<!-- envguard-example excerpt -->
```text
$ envguard check --env production
.env.production
        ✗ error    EG001  STRIPE_SECRET_KEY is declared in .env.production.example but missing from .env.production
```

Fix: add `STRIPE_SECRET_KEY=` to `.env.production`. If production really does
not need it, mark it optional in the template (`STRIPE_SECRET_KEY= # optional`).

### The code reads a variable nobody declared

`process.env.ANALYTICS_WRITE_KEY` is read in `src/server.js`. No env file,
template, Compose file or workflow mentions it, so nobody knows to set it.

<!-- envguard-file path=src/server.js -->
```js
  const analytics = process.env.ANALYTICS_WRITE_KEY;
```

<!-- envguard-example excerpt -->
```text
$ envguard check
src/server.js
  23:33  ✗ error    EG003  ANALYTICS_WRITE_KEY is read by the code but not declared in any env file, template or schema
```

Fix: add `ANALYTICS_WRITE_KEY=` to `.env.example`, then set it where you deploy.

### A real `.env` is committed to git

Anyone who can read the repository can read the values, and they stay in git
history even after the file is deleted.

<!-- envguard-example excerpt -->
```text
$ envguard check
.env
        ✗ error    EG008  .env is a real env file and is committed to git
```

Fix: `git rm --cached .env`, add `.env` to `.gitignore`, and **rotate every
secret in it**. `envguard doctor` prints these steps with the exact file names.

### A secret sits in `.env.example`

Templates are shared with everyone, so a real credential there is a leak.
EnvGuard spots the password in this URL. It never prints the value.

<!-- envguard-file path=.env.example -->
```dotenv
DATABASE_URL=postgres://app:demo-only-pa55word@db.internal:5432/app
```

<!-- envguard-example excerpt -->
```text
$ envguard check
.env.example
  4:14  ✗ error    EG010  DATABASE_URL in .env.example looks like a URL with an embedded password
```

Fix: use an empty value or a placeholder (`DATABASE_URL=postgres://user:password@localhost/app`).
Local credentials and obvious placeholders are not flagged.

### A Compose variable is undefined

Compose replaces an unset `${POSTGRES_PASSWORD}` with an empty string and only
prints a warning. The database container then starts with a broken password.

<!-- envguard-file path=docker-compose.yml -->
```yaml
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
```

<!-- envguard-example excerpt -->
```text
$ envguard check
docker-compose.yml
  17:28  ⚠ warning  EG011  docker-compose.yml interpolates ${POSTGRES_PASSWORD}, which is not defined in .env (the file Compose reads) or any template
```

Fix: define it in `.env` and document it in a template, or give it a default
(`${POSTGRES_PASSWORD:-value}`).

### Staging and production drifted apart

Each environment has its own template. A variable that only one of them
declares is usually one somebody forgot to add.

<!-- envguard-example excerpt -->
```text
$ envguard check
.env.production.example
  2:1  ⚠ warning  EG013  STRIPE_SECRET_KEY is declared for production but not for staging
.env.staging.example
  2:1  ⚠ warning  EG013  STAGING_BANNER is declared for staging but not for production
```

`envguard diff` shows the whole picture, with names only and never values:

<!-- envguard-example -->
```text
$ envguard diff staging production
Comparing staging → production

App .
  staging: .env.example, .env.staging.example (templates)
  production: .env.production
  - LEGACY_API_KEY  only in staging
  - SENTRY_DSN      only in staging
  - STAGING_BANNER  only in staging
  8 variables in both

3 variables differ in 1 app root.
```

<details>
<summary>The whole <code>envguard check</code> output for the demo app</summary>

<!-- envguard-example exit=1 -->
```text
$ envguard check
.env
        ✗ error    EG008  .env is a real env file and is committed to git
  2:11  ✗ error    EG014  LOG_LEVEL in .env has the invalid value verbose: expected one of debug, info, warn or error
   5:1  ⚠ warning  EG005  SESSION_SECRET is empty in .env but required by .env.example
   8:1  ⚠ warning  EG002  FEATURE_NEW_CHECKOUT is set in .env but not documented in any template
   9:1  ⚠ warning  EG002  CACHE_DIR is set in .env but not documented in any template
  9:11  ⚠ warning  EG016  CACHE_DIR references APP_HOME, which is not defined in .env or the env files loaded with it

.env.example
  4:14  ✗ error    EG010  DATABASE_URL in .env.example looks like a URL with an embedded password
   9:1  • info     EG004  LEGACY_API_KEY is declared in .env.example but never read by the code, env files, Compose files or workflows

.env.production
        ✗ error    EG001  STRIPE_SECRET_KEY is declared in .env.production.example but missing from .env.production
        ✗ error    EG008  .env.production is a real env file and is committed to git
  4:11  ⚠ warning  EG015  REDIS_URL points to a local address in production (.env.production)
   6:9  ✗ error    EG014  WORKERS in .env.production has the invalid value 0: is below the minimum 1
   8:1  ⚠ warning  EG006  DEBUG is defined 2 times in .env.production (lines 7 and 8)
   8:7  ⚠ warning  EG015  DEBUG is enabled in production (.env.production)
  10:8  ✗ error    EG007  missing '=' after key

.env.production.example
  2:1  ⚠ warning  EG013  STRIPE_SECRET_KEY is declared for production but not for staging

.env.staging.example
  2:1  ⚠ warning  EG013  STAGING_BANNER is declared for staging but not for production

.github/workflows/deploy.yml
  14:30  • info     EG012  workflows use secret FLY_API_TOKEN, but no template or schema documents it
  15:30  • info     EG012  workflows use variable DEPLOY_REGION, but no template or schema documents it

docker-compose.yml
  17:28  ⚠ warning  EG011  docker-compose.yml interpolates ${POSTGRES_PASSWORD}, which is not defined in .env (the file Compose reads) or any template

src/server.js
  23:33  ✗ error    EG003  ANALYTICS_WRITE_KEY is read by the code but not declared in any env file, template or schema

worker/main.py
  6:30  • info     EG003  WORKER_CONCURRENCY is read by the code but not declared in any env file, template or schema

✗ 8 errors · ⚠ 10 warnings · 4 info · 10 files
Run `envguard doctor` to see why each problem matters and how to fix it, or `envguard explain EG008`.
```

</details>

Every finding says what is wrong, why it matters and how to fix it.
`envguard doctor` shows all three, and `envguard explain EG003` shows the full
documentation of a rule.

## Use it in CI

**GitHub Actions.** Findings become annotations on the pull request, and the
step fails on errors:

```yaml
steps:
  - uses: actions/checkout@v5
  - uses: ardamoustafa1/envguard@v1
```

> Screenshot placeholder: pull request annotations (`docs/images/pr-annotations.png`).

Inputs for a version pin, `--env`, `fail-on` and a SARIF upload to code
scanning are in [docs/ci.md](docs/ci.md), together with GitLab CI and CircleCI.

**pre-commit:**

```yaml
repos:
  - repo: https://github.com/ardamoustafa1/envguard
    rev: v0.1.0
    hooks:
      - id: envguard
```

**Docker entrypoint.** `--process-env` checks the environment the container
actually received, against your templates and schema. The container stops
before your app starts if a required variable is unset, empty or invalid:

```dockerfile
COPY --from=ghcr.io/ardamoustafa1/envguard /usr/local/bin/envguard /usr/local/bin/envguard
COPY .env*.example .envguard.yaml /app/
ENTRYPOINT ["/bin/sh", "-c", "envguard check --root /app --process-env --env production --quiet && exec \"$@\"", "--"]
CMD ["node", "server.js"]
```

Drop `--quiet` to see the findings in the container logs. Values are never printed.

Output formats: `--format pretty|doctor|json|sarif|github` and `--output FILE`.
The JSON schema is documented and versioned: [docs/json-output.md](docs/json-output.md).

## Zero config, optional power

Everything above works with no setup. To go further, describe your variables
in `.envguard.yaml`. `envguard init` writes a starter file for you.

<!-- envguard-config -->
```yaml
version: 1
profiles: [production-safety]     # flags DEBUG=true, NODE_ENV!=production, localhost URLs in production
variables:
  DATABASE_URL:
    type: url
    schemes: [postgres, postgresql]
    required: true
    sensitive: true
  LOG_LEVEL: { type: enum, values: [debug, info, warn, error] }
  WORKERS: { type: int, min: 1, max: 64 }
```

Types, constraints, per-environment requirements, ignores, rule severities
and inline `# envguard-ignore` comments: [docs/configuration.md](docs/configuration.md).
A JSON Schema ([docs/envguard.schema.json](docs/envguard.schema.json)) gives
your editor autocompletion.

## Rules

<!-- rules-table:start -->
| Rule | Name | Severity | Catches |
|---|---|---|---|
| [EG001](docs/rules/EG001-missing-variable.md) | `missing-variable` | error | A variable declared in a template or required by the schema is missing from an environment's real env file. |
| [EG002](docs/rules/EG002-undocumented-variable.md) | `undocumented-variable` | warning | A real env file sets a variable that no template or schema documents. |
| [EG003](docs/rules/EG003-used-but-undeclared.md) | `used-but-undeclared` | error | The code reads an environment variable that no env file, template, schema, Compose file or workflow declares. |
| [EG004](docs/rules/EG004-declared-but-unused.md) | `declared-but-unused` | info | A template declares a variable that no code, env file reference, Compose file or workflow reads. |
| [EG005](docs/rules/EG005-empty-required-value.md) | `empty-required-value` | warning | A required variable is present but empty in a real env file or the process environment. |
| [EG006](docs/rules/EG006-duplicate-key.md) | `duplicate-key` | warning | The same key is defined more than once in one env file or template. |
| [EG007](docs/rules/EG007-syntax-error.md) | `syntax-error` | error | An env file, Compose file or workflow cannot be parsed, or an env file line cannot be parsed reliably. |
| [EG008](docs/rules/EG008-env-file-committed.md) | `env-file-committed` | error | A real env file (not a template) is tracked by git. |
| [EG009](docs/rules/EG009-env-file-not-ignored.md) | `env-file-not-ignored` | warning | A real env file is not tracked by git, but no `.gitignore` rule covers it. |
| [EG010](docs/rules/EG010-possible-secret-in-template.md) | `possible-secret-in-template` | error | A template, or a real env file that git tracks, contains a value that looks like a real credential. |
| [EG011](docs/rules/EG011-compose-undefined-variable.md) | `compose-undefined-variable` | warning | A Compose file interpolates an undefined variable without a default, or a required `env_file` does not exist. |
| [EG012](docs/rules/EG012-ci-secret-undocumented.md) | `ci-secret-undocumented` | info | A GitHub Actions workflow references `secrets.X` or `vars.X` that no template or schema documents. |
| [EG013](docs/rules/EG013-environment-drift.md) | `environment-drift` | warning | Templates of different environments of the same app disagree on which variables exist. |
| [EG014](docs/rules/EG014-invalid-value.md) | `invalid-value` | error | A value in a real env file or the process environment violates its schema in `.envguard.yaml`. |
| [EG015](docs/rules/EG015-unsafe-production-value.md) | `unsafe-production-value` | warning | A production value looks like a development setting; active only via schema `production:` rules or a profile. |
| [EG016](docs/rules/EG016-unresolved-reference.md) | `unresolved-reference` | warning | A `$VAR` or `${VAR}` reference inside an env file points to a variable that nothing defines. |
<!-- rules-table:end -->

`envguard explain <RULE>` prints a rule's documentation. Rule IDs never change.

## Safe by design

EnvGuard reads files that often hold credentials, so its safety properties
are enforced by tests, not just promised.

| Promise | How it is enforced |
|---|---|
| **Offline.** No network calls, no telemetry, no update check. | `TestOffline` runs `go list -deps` and fails if `net/http` or any other network client is in the dependency graph. |
| **Read-only.** It never changes your files. | `TestCommandsDoNotModifyTheRepository` hashes a repository before and after every command. `TestOnlyDocumentedFilesWriteToDisk` fails if any other source file can write. The only writes are `envguard init`, `--output FILE` and the GitHub job summary. |
| **Secrets are always masked.** | Values reach output only through one package, `internal/mask`. `TestLeakCanary` fills a repository with unique fake secrets, runs every command, format, flag and error path, and fails if a single one appears in any output or file. |
| **Hostile repositories are safe to analyze.** Cloned code cannot crash EnvGuard or fake its output. | `TestHostileRepositoryNeverCrashesOrInjects` runs every command and format on a repository of huge, binary, deeply nested, invalid-UTF-8 and symlinked files, with file names that contain terminal escape sequences. It fails on a crash, a panic dump, a stalled run, or any control character in text output. Parsers and the scanner are fuzzed (`make fuzz`), and `TestNoHiddenCharactersInSources` keeps hidden characters out of the source itself. |
| **Same input, same output.** | Findings are sorted deterministically. Golden tests cover every format, and a test renders each twice. |
| **No code runs from your repository.** | Git is called with fixed arguments, no shell, and with `fsmonitor` disabled, which would otherwise run a program from a repo's git config (`TestInspectIgnoresMaliciousFsmonitor`). |

Release downloads are checksummed, and the checksums are signed with Sigstore:
[how to verify](docs/installation.md#verifying-downloads).

## EnvGuard and other tools

EnvGuard checks that your *configuration agrees with itself and with your
code*. It does not store, encrypt, sync or rotate secrets. It is not a full
secret scanner. It works well next to these tools:

| Tool | What it does best | Use EnvGuard when |
|---|---|---|
| [dotenv-linter](https://github.com/dotenv-linter/dotenv-linter) | Style of `.env` files: key order, spacing, quotes. | You care about whether files *agree* with each other and with your code. EnvGuard finds duplicates and syntax errors too, but has no style rules. |
| [dotenvx](https://github.com/dotenvx/dotenvx) | Encrypting `.env` files and loading them at run time. | You want to know your variables are *complete and valid*. Use both: dotenvx protects the values, EnvGuard checks the names. |
| [gitleaks](https://github.com/gitleaks/gitleaks) | Finding secrets in git history and arbitrary files, with a large rule set. | Never instead of it. EnvGuard only inspects templates and tracked env files, and does not scan history. **Run gitleaks too**, especially for history. |
| [envalid](https://github.com/af/envalid), [t3-env](https://github.com/t3-oss/t3-env) | Validating `process.env` inside your Node app at startup, with typed access. | You want a check *before deploy*, across Compose, CI and templates, and for non-Node code. They can still be your runtime guard. EnvGuard's schema is separate from theirs. |
| [Infisical](https://github.com/Infisical/infisical) | A secrets platform: storage, sync, access control, rotation. | You want a free offline check that what you deploy matches what the code needs. Run `envguard check --process-env` on the environment a secrets manager injects. |

## FAQ

**Does EnvGuard send anything anywhere?** No. It has no network code, and a
test enforces that. Installing it downloads files; running it does not.

**Does it print my secrets?** No. Messages say *which* variable has a problem,
never its value. Values it does show (short, non-sensitive ones) pass through
one masking function. `TestLeakCanary` guards this.

**Do I need git?** No. Without git, the checks that need it (EG008, EG009) are
skipped and `--verbose` says so.

**Which languages does it understand?** JavaScript/TypeScript (including Vue
and Svelte), Python, Go, Ruby, PHP, Rust, Java, Kotlin, C#, Elixir and shell.
It prefers missing a usage over inventing one. Computed names such as
`process.env[name]` are counted but cannot be analyzed.
Details and limits: [docs/how-it-works.md](docs/how-it-works.md).

**A finding is wrong. What do I do?** Silence it with `# envguard-ignore EG002`
above the line, or in `.envguard.yaml` with a reason. If it is a false
positive, please [open an issue](https://github.com/ardamoustafa1/envguard/issues/new/choose)
with a minimal snippet and **no real values**. Low false positives matter more
to us than more rules.

**Does it work in a monorepo?** Yes. Directories with a `package.json`,
`go.mod`, `pyproject.toml` and similar markers are app roots. Use `--app apps/api`
to look at one.

**Does it work on Windows?** Yes. CI runs on Linux, macOS and Windows, and
releases include Windows binaries. Add `--ascii` if your terminal cannot show
Unicode.

**Can I add a rule or a language?** Yes, and these are the best first
contributions. See [CONTRIBUTING.md](CONTRIBUTING.md).

## Contributing

Bug reports, false positives, new rules and new language detectors are all
welcome. Start with [CONTRIBUTING.md](CONTRIBUTING.md). It has step-by-step
guides for adding a rule and a language detector, and the
[good first issues](docs/roadmap/good-first-issues.md) list. Where the project
is heading: [docs/roadmap.md](docs/roadmap.md). Please read the
[Code of Conduct](CODE_OF_CONDUCT.md), and report security problems privately
as described in [SECURITY.md](SECURITY.md).

```bash
make build   # bin/envguard
make test    # go test -race ./...
make lint
make demo    # re-record docs/demo.gif (needs vhs)
```

## License

[MIT](LICENSE)

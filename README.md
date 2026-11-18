<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/logo-dark.svg">
    <img src="docs/logo.svg" alt="EnvGuard" width="320" height="77">
  </picture>
</p>

<h1 align="center">Catch missing environment variables before deployment</h1>

<p align="center">
  Check that your code, <code>.env</code> files, Docker Compose, and GitHub Actions agree on which variables your app needs.
</p>

<p align="center">
  <a href="#get-started">Get started</a> ·
  <a href="#codex-and-claude-code">Codex &amp; Claude Code</a> ·
  <a href="#github-actions">GitHub Actions</a> ·
  <a href="https://github.com/ardamoustafa1/envguard/releases/latest">Download</a>
</p>

<p align="center">
  <a href="https://github.com/ardamoustafa1/envguard/actions/workflows/ci.yml"><img src="https://github.com/ardamoustafa1/envguard/actions/workflows/ci.yml/badge.svg" alt="CI status"></a>
  <a href="https://github.com/ardamoustafa1/envguard/releases/latest"><img src="https://img.shields.io/github/v/release/ardamoustafa1/envguard" alt="Latest release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/ardamoustafa1/envguard" alt="MIT license"></a>
</p>

## Get started

[Download the binary](https://github.com/ardamoustafa1/envguard/releases/latest)
for macOS, Linux, or Windows and put it on your `PATH`. Then run this in your project:

```sh
envguard version
envguard check
```

If you have Go 1.26 or newer, you can install with
`go install github.com/ardamoustafa1/envguard/cmd/envguard@v0.1.0`.
See the [installation guide](docs/installation.md) for Docker, Linux packages,
and signature verification.

| Where you work | How to use EnvGuard |
|---|---|
| Terminal | Run `envguard check` in a project. |
| Codex or Claude Code | [Install the skill once](#codex-and-claude-code) for all your local projects. |
| Pull requests | [Add a GitHub Actions step](#github-actions) to check every change. |

## Codex and Claude Code

Install the EnvGuard CLI first. With [GitHub CLI](https://cli.github.com/) and
`gh skill` available, run the command for your agent once from any directory.
User scope makes the skill available in every local project.

**Codex**

```sh
gh skill install ardamoustafa1/envguard skills/envguard --pin main --agent codex --scope user
```

**Claude Code**

```sh
gh skill install ardamoustafa1/envguard skills/envguard --pin main --agent claude-code --scope user
```

Open a new session in a project and ask **“Check this project's environment
variables with EnvGuard.”** In Codex, mention `$envguard`. In Claude Code,
run `/envguard`. The skill calls the local CLI, so no MCP server is required.

<details>
<summary>Check automatically after configuration changes</summary>

Add this instruction to your user-level `~/.codex/AGENTS.md` or a Markdown
file in `~/.claude/rules/`:

> After changing environment variable names, env templates or schemas, or
> variable references in CI or Docker Compose, use the EnvGuard skill to
> check the affected project and summarize the findings without quoting
> environment variable values.

</details>

## What it catches

A production template declares `STRIPE_SECRET_KEY`, but the production env
file is missing it. EnvGuard points to the missing setting before deployment:

<!-- envguard-example excerpt -->
```text
$ envguard check --env production
.env.production
        ✗ error    EG001  STRIPE_SECRET_KEY is declared in .env.production.example but missing from .env.production
```

| Problem | What EnvGuard checks |
|---|---|
| Missing or undocumented variables | Compares code references and env files with templates and the optional schema. |
| Configuration drift | Finds differences between environments and apps in a monorepo. |
| Possible credential exposure | Flags tracked env files and likely credentials in env templates. |
| Broken deployment configuration | Finds undefined Compose references, undocumented GitHub Actions variables, and invalid values. |

<p align="center">
  <img src="docs/demo.gif" alt="Terminal demo of EnvGuard checking a project, explaining a rule, and comparing environments" width="860">
</p>

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

Use `envguard doctor` for suggested fixes, `envguard explain EG003` for a
rule's details, or `envguard diff staging production` to compare environments.
The output above comes from the [demo project](examples/demo-app) and is checked in CI.

## GitHub Actions

Add EnvGuard after checkout in a workflow job. It annotates findings on pull
requests and fails the step when it finds errors.

```yaml
steps:
  - uses: actions/checkout@v5
  - uses: ardamoustafa1/envguard@v0.1.0
```

See the [CI guide](docs/ci.md) for GitHub Actions inputs, SARIF reports,
pre-commit hooks, Docker entrypoints, and other CI platforms.

## Configuration

EnvGuard discovers supported files on its own. Add an optional
`.envguard.yaml` when you need types, required variables, or per-environment rules.

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


See the [configuration reference](docs/configuration.md) and
[supported languages and limitations](docs/how-it-works.md).

<details>
<summary>All 16 rules</summary>

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

</details>

## Privacy and verification

A standard check runs offline, makes no changes, and does not execute your
application. EnvGuard masks recognized credentials and values marked
`sensitive` in the schema. You can
[verify downloads with Sigstore](docs/installation.md#verifying-downloads).

<details>
<summary>How the project tests these properties</summary>

| Promise | How it is enforced |
|---|---|
| **Offline.** No network calls, no telemetry, no update check. | `TestOffline` runs `go list -deps` and fails if `net/http` or any other network client is in the dependency graph. |
| **Read-only.** It never changes your files. | `TestCommandsDoNotModifyTheRepository` hashes a repository before and after every command. `TestOnlyDocumentedFilesWriteToDisk` fails if any other source file can write. The only writes are `envguard init`, `--output FILE` and the GitHub job summary. |
| **Credential masking.** | Values reach output only through one package, `internal/mask`. `TestLeakCanary` fills a repository with unique fake secrets, runs every command, format, flag and error path, and fails if a single one appears in any output or file. |
| **Resilience.** The scanner is tested against malformed repositories. | `TestHostileRepositoryNeverCrashesOrInjects` runs every command and format on a repository of huge, binary, deeply nested, invalid-UTF-8 and symlinked files, with file names that contain terminal escape sequences. It fails on a crash, a panic dump, a stalled run, or any control character in text output. Parsers and the scanner are fuzzed (`make fuzz`), and `TestNoHiddenCharactersInSources` keeps hidden characters out of the source itself. |
| **Same input, same output.** | Findings are sorted deterministically. Golden tests cover every format, and a test renders each twice. |
| **No code runs from your repository.** | Git is called with fixed arguments, no shell, and with `fsmonitor` disabled, which would otherwise run a program from a repo's git config (`TestInspectIgnoresMaliciousFsmonitor`). |

</details>

## Contributing

Start with the [contributor guide](CONTRIBUTING.md) or
[good first issues](docs/roadmap/good-first-issues.md). See the
[roadmap](docs/roadmap.md) for planned work and the
[security policy](SECURITY.md) for private vulnerability reporting.

EnvGuard is open source under the [MIT license](LICENSE).
[Browse the changelog](CHANGELOG.md) or [report an issue](https://github.com/ardamoustafa1/envguard/issues/new/choose).

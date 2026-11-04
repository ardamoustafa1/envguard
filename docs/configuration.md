# Configuration

EnvGuard works without any configuration. A file named `.envguard.yaml` adds
power: a schema for your variables, severity overrides, suppressions and
environment mappings.

Generate a starting point from your repository:

```sh
envguard init              # writes .envguard.yaml
envguard init --example    # also writes .env.example for the app root
envguard init --dry-run    # print the files instead of writing them
```

`init` refuses to overwrite existing files unless you pass `--force`. It never
copies a value from any env file. It infers types from names (`*_PORT` is a
`port`, `*_URL` is a `url`) and from template samples, and marks variables as
`required` when code reads them without a default.

## File lookup and precedence

EnvGuard looks for the configuration in this order:

1. The file given with `--config`. It must exist.
2. `<root>/.envguard.yaml`
3. `<root>/.envguard.yml`

If both the `.yaml` and the `.yml` file exist, that is an error. If none
exists, defaults apply. The file is limited to 1 MiB.

Settings are merged with this precedence: **flags > config file > defaults**. A
flag overrides the file only when you pass it. Today `--fail-on` is the flag
that has a config counterpart (`fail_on`).

## Editor autocompletion

The repository ships a JSON Schema at `docs/envguard.schema.json`. Point your
editor at it with a comment on the first line (the
[YAML language server](https://github.com/redhat-developer/yaml-language-server)
reads it; VS Code uses it through the YAML extension):

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/ardamoustafa1/envguard/main/docs/envguard.schema.json
version: 1
```

## Strict validation

The file is validated strictly. Every problem is reported at once, in file
order, with `file:line:col`, and EnvGuard exits with code 2:

```text
envguard: invalid configuration: 3 problems in .envguard.yaml
  .envguard.yaml:3:1: unknown key "fail_one" in the top level (did you mean "fail_on"?)
  .envguard.yaml:7:9: invalid value for "rules.EG004"; allowed: error, warning, info, off (did you mean "warning"?)
  .envguard.yaml:12:11: invalid value for "variables.PORT.type"; allowed: string, int, float, bool, url, port, email, hostname, duration, enum, json, regex (did you mean "int"?)
```

What is rejected: unknown keys and duplicate keys (with a "did you mean"
suggestion), wrong YAML types, unknown enum values, invalid globs, invalid
regular expressions, and inconsistent variable rules (see
[variables](#variables)). Messages name keys, never values, so they are safe to
paste into CI logs.

## Top-level keys

| Key | Type | Default | Purpose |
|---|---|---|---|
| `version` | integer | required | Must be `1`. |
| `fail_on` | string | `error` | Exit threshold: `error`, `warning`, `info` or `never`. |
| `include` | list of globs | all files | Analyze only matching files. |
| `exclude` | list of globs | none | Skip matching files and directories. |
| `environments` | map | inferred | Map files to environment names. |
| `rules` | map | defaults | Change a rule's severity or turn it off. |
| `ignore` | list | none | Suppress findings. |
| `scanner` | map | all languages | Source scanner settings. |
| `profiles` | list | none | Opt-in rule packs. |
| `variables` | map | none | The schema of your variables. |

### version

```yaml
version: 1
```

Required. Any other value is an error.

### fail_on

The lowest severity that makes `envguard check` exit with 1. The default is
`error`. `never` always exits 0 unless EnvGuard itself fails.

```yaml
version: 1
fail_on: warning
```

The `--fail-on` flag wins over this key. `envguard doctor` ignores `fail_on`;
only `doctor --fail-on` makes it fail.

### include and exclude

Glob lists that limit what EnvGuard looks at. Globs use
[doublestar](https://github.com/bmatcuk/doublestar) syntax (`**` matches any
number of directories) and apply to paths **relative to the root**, with forward
slashes, also on Windows.

A pattern **without a `/` matches the base name at any depth**. A pattern with
a `/` matches the whole relative path.

| Pattern | Matches |
|---|---|
| `*.test.js` | `a.test.js`, `src/deep/b.test.js` |
| `fixtures` | a file or directory named `fixtures`, anywhere |
| `examples/**` | everything below the top-level `examples/` |
| `apps/*/legacy` | `apps/web/legacy`, not `apps/web/x/legacy` |

Patterns must be relative: no leading `/`, no `../`, no backslashes, not
empty.

```yaml
version: 1
include:
  - "apps/**"
  - "packages/**"
exclude:
  - "**/*.generated.ts"
  - testdata
```

`exclude` adds to what is already skipped: `.gitignore` files and the built-in
directories (`.git`, `node_modules`, `vendor`, `dist`, `build`, `target`,
`.venv`, `venv`, `__pycache__`, `.next`, `.nuxt`, `coverage`). When `include`
is set, only matching files are analyzed. See
[How it works](how-it-works.md#discovery) for the full walk rules.

### environments

EnvGuard infers an environment from the file name (`.env.production` is
`production`). Use `environments` for files whose names it cannot infer. Each
entry needs `files` (real env files), `templates`, or both. Matches here win
over name inference.

```yaml
version: 1
environments:
  production:
    files: ["deploy/prod.env"]
    templates: ["deploy/prod.env.tpl"]
  staging:
    files: ["deploy/staging.env"]
```

Environment names are lower-case letters, digits, `-` and `_`, starting with a
letter or digit.

### rules

Change the severity of a rule by ID, or turn it off with `off`. Values:
`error`, `warning`, `info`, `off`.

```yaml
version: 1
rules:
  EG004: off        # do not report declared-but-unused variables
  EG002: error      # undocumented variables break the build
  EG012: warning
```

Keys must look like `EG004`. A rule set to `off` does not run. Run
`envguard explain --list` to see the rules and their default severities.

### ignore

Suppress findings that you have reviewed. Every entry must set `path` and/or
`variable`. To disable a rule everywhere, use `rules` instead.

| Field | Meaning |
|---|---|
| `rule` | Rule ID, such as `EG004`. Omit to match every rule. |
| `path` | Glob of files, relative to the root. |
| `variable` | Variable name. `*` matches any characters. |
| `reason` | Free text for reviewers. EnvGuard does not use it. |

All fields you set must match. Suppressed findings are counted in the summary
(`3 suppressed`).

```yaml
version: 1
ignore:
  - rule: EG004
    variable: LEGACY_*
    reason: Read by the old billing service, removed in Q3.
  - rule: EG002
    path: "**/.env.local"
  - variable: SENTRY_DSN
    reason: Injected by the platform.
```

You can also suppress inside env files; see
[inline directives](#inline-directives-in-env-files).

### scanner

| Field | Meaning |
|---|---|
| `languages` | Languages to scan. Default: all. |
| `ignore_variables` | Variable names the scanner should ignore. `*` matches any characters. |

Languages: `csharp`, `elixir`, `go`, `java`, `javascript`, `kotlin`, `php`,
`python`, `ruby`, `rust`, `shell`, `svelte`, `typescript`, `vue`.

```yaml
version: 1
scanner:
  languages: [typescript, python, go]
  ignore_variables:
    - GITHUB_*
    - AWS_LAMBDA_*
    - PORT
```

Use `ignore_variables` for variables the platform provides. EnvGuard already
skips well-known ones such as `PATH`, `HOME`, `CI`, `NODE_ENV` and `GITHUB_*`.

### profiles

Opt-in rule packs. There is one: `production-safety`.

```yaml
version: 1
profiles: [production-safety]
```

It enables built-in checks of rule EG015 (`unsafe-production-value`) on the
`production` environment. It only looks at real env files of that environment
(and at the process environment with `--process-env`) and skips empty values:

| Variable | Reported when |
|---|---|
| `DEBUG`, `DJANGO_DEBUG`, `APP_DEBUG`, `FLASK_DEBUG` | The value is truthy (`true`, `1`, `yes`, `on`). |
| `NODE_ENV`, `RAILS_ENV` | The value is not `production`. |
| `APP_ENV` | The value is `local`, `development` or `dev`. |
| Names ending in `_URL` or `_HOST` | The host is `localhost`, `127.0.0.1`, `0.0.0.0` or `::1`. |

With `--process-env`, the `_URL` and `_HOST` check applies only to variables
declared in a template or the schema. If a variable has a `production:` rule
in the schema, that rule replaces the built-in check for it.

## variables

The schema. Keys are variable names (letters, digits, `_`, `.`, `-`; not
starting with a digit). Every field is optional.

```yaml
version: 1
variables:
  DATABASE_URL:
    description: Primary Postgres connection string.
    type: url
    schemes: [postgres, postgresql]
    required: true
    sensitive: true
```

| Field | Type | Meaning |
|---|---|---|
| `description` | string | What the variable is for. |
| `type` | string | Value type. Default `string`. See [types](#types). |
| `required` | bool | Must be set. Default `false`. Checked by EG001 and EG005. |
| `environments` | list | Environments where `required` applies. Default: all. Needs `required: true`. |
| `sensitive` | bool | Always mask the value in output. |
| `default` | string | Documented default. Must be valid for the type. |
| `values` | list | Allowed values. Required for `enum`, only allowed there. |
| `schemes` | list | Allowed URL schemes. `url` only. |
| `pattern` | string | Go (RE2) regular expression. It is **searched**, so anchor with `^` and `$`. |
| `min`, `max` | number | Numeric bounds. `int`, `float`, `port` only. |
| `min_length`, `max_length` | integer | Length in characters. Any type. |
| `production` | map | Extra constraints in production (EG015). |

Schema variables count as documented (EG002) and declared (EG003). A
`required: true` variable is expected only in app roots that reference it (in
a template or in code), in the root app, and in `--process-env` mode, so a
monorepo-wide schema does not demand every variable in every app.

Values are checked by EG014 on the last definition of each key in real env
files and in the process environment. Templates are never validated, and empty
values are skipped (EG005 handles those).

### Types

| Type | A value is valid when |
|---|---|
| `string` | Always. The default. |
| `int` | It parses as a base-10 integer (surrounding blanks allowed). `4.2` is invalid. |
| `float` | It parses as a finite number. `1e3` is valid, `NaN` and `Inf` are not. |
| `bool` | It is `true`, `false`, `1`, `0`, `yes`, `no`, `on` or `off`, case-insensitive. |
| `url` | It has a scheme and a host or path. `https://x.io` and `sqlite:///data/app.db` pass; `localhost:5432` and `api.example.com` do not. |
| `port` | It is an integer from 1 to 65535. |
| `email` | It looks like `name@host.tld`, with no spaces. `ops@localhost` is invalid. |
| `hostname` | It is an RFC 1123 hostname (labels of letters, digits and `-`) or an IP address. |
| `duration` | It uses Go syntax: `300ms`, `30s`, `1h30m`. A bare `30` is invalid. |
| `enum` | It equals one of `values`, case-sensitive. |
| `json` | It is valid JSON. |
| `regex` | It compiles as a Go regular expression. |

### Which constraints apply to which type

| Constraint | Types |
|---|---|
| `values` | `enum` only (and required there) |
| `schemes` | `url` only |
| `min`, `max` | `int`, `float`, `port` |
| `min_length`, `max_length` | all |
| `pattern` | all |
| `default`, `required`, `sensitive`, `description` | all |

`min` must not exceed `max`, and `min_length` must not exceed `max_length`.
Lengths cannot be negative.

### production

Rules for the `production` environment, checked by EG015 only. Set at least
one of:

| Field | Meaning |
|---|---|
| `must_be` | The value must equal this. Compared by type, so `off` equals `false`. |
| `must_not_be` | The value must not equal this. |
| `pattern` | The value must match this regular expression. |

`must_be` and `must_not_be` must be valid for the type, and they cannot be
equal.

```yaml
version: 1
variables:
  DEBUG:
    type: bool
    production:
      must_be: "false"
  LOG_LEVEL:
    type: enum
    values: [debug, info, warn, error]
    production:
      must_not_be: debug
  API_BASE_URL:
    type: url
    production:
      pattern: "^https://"
```

## Inline directives in env files

You can suppress findings next to the code they concern, in any env file.
Rule IDs are case-insensitive and separated by commas or spaces. Text after
`--` is a free-form reason.

```sh
# envguard-ignore EG002 -- set by the platform, not documented on purpose
LEGACY_FLAG=1

DEBUG_SQL=1 # envguard-ignore EG002,EG015
```

| Form | Applies to |
|---|---|
| `# envguard-ignore EG002` on its own line | The next assignment line (even a malformed one). Stacked directives merge. |
| `KEY=value # envguard-ignore EG005` at the end of a line | That entry. |
| `# envguard-ignore-file EG002` | The whole file. |

A directive also matches a finding that merely points to the line (for
example "missing" findings listed under the template line), so
`# envguard-ignore EG001` above a template line silences it. A directive with
a missing or invalid rule ID is reported as a warning.

## Template conventions

Templates (`.env.example`, `.env.sample`, `.env.template`, `.env.dist`,
`.env.defaults`, `.env.production.example`) declare what a real file must
contain. A template variable is **required** if it has a non-empty value or if
its line is not marked optional. Only an empty value with an `optional` comment
is optional:

```sh
DATABASE_URL=                 # required: no marker
PORT=3000                     # required: has a value
SENTRY_DSN=                   # optional
FEATURE_X=                    # optional: off unless set
CDN_URL=                      # optional - falls back to the app host
```

The comment can read `optional`, `optional: ...` or `optional - ...`,
case-insensitive. EG001 (missing) and EG005 (empty) skip optional variables.

For a real file, the applicable templates are the app's default templates plus
the templates of its own environment: `.env.production` is checked against
`.env.example` and `.env.production.example`.

## A complete example

<!-- envguard-config -->
```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/ardamoustafa1/envguard/main/docs/envguard.schema.json
version: 1

# Fail the build on warnings too.
fail_on: warning

exclude:
  - testdata
  - "**/*.generated.ts"

environments:
  production:
    files: ["deploy/prod.env"]
  staging:
    files: ["deploy/staging.env"]

profiles: [production-safety]

rules:
  EG004: info
  EG012: off

ignore:
  - rule: EG004
    variable: LEGACY_*
    reason: Read by the old billing service until it is retired.
  - rule: EG002
    path: "scripts/**"

scanner:
  languages: [typescript, python]
  ignore_variables:
    - AWS_LAMBDA_*
    - PORT

variables:
  DATABASE_URL:
    description: Primary Postgres connection string.
    type: url
    schemes: [postgres, postgresql]
    required: true
    sensitive: true

  PORT:
    description: HTTP port the API listens on.
    type: port
    default: "3000"

  LOG_LEVEL:
    description: Minimum log level.
    type: enum
    values: [debug, info, warn, error]
    default: info
    production:
      must_not_be: debug

  DEBUG:
    description: Enables verbose debug output.
    type: bool
    default: "false"
    production:
      must_be: "false"

  REQUEST_TIMEOUT:
    description: Upstream request timeout.
    type: duration
    default: 30s

  WORKER_COUNT:
    description: Number of background workers.
    type: int
    min: 1
    max: 64
    default: "4"

  API_BASE_URL:
    description: Public base URL of the API.
    type: url
    schemes: [https]
    required: true
    environments: [production, staging]

  JWT_SECRET:
    description: Signing key for session tokens.
    required: true
    sensitive: true
    min_length: 32
```

See [JSON output](json-output.md), [CI](ci.md) and the rule pages in
[docs/rules](rules/README.md).

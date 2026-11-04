# How EnvGuard works

EnvGuard reads your repository, builds one immutable model of it, and runs
pure rules over that model. It is offline, read-only, and never prints secret
values.

## The pipeline

```mermaid
flowchart TD
    A[flags and .envguard.yaml] --> B[config]
    B --> C[discovery]
    C --> D1[parser: env files and templates]
    C --> D2[scanner: source code]
    C --> D3[compose]
    C --> D4[workflow]
    C --> D5[gitutil]
    B --> D6[schema: variables]
    D1 --> E[project model]
    D2 --> E
    D3 --> E
    D4 --> E
    D5 --> E
    D6 --> E
    E --> F[rules]
    F --> G[suppression, severity overrides, sort]
    G --> H[reporters]
    M[mask] --> H
    H --> I[stdout or --output file]
    G --> J[exit code from --fail-on]
```

1. **Config.** Flags, `.envguard.yaml` and defaults are merged
   ([configuration](configuration.md)).
2. **Discovery** walks the repository and classifies files.
3. **Parsers** read env files and templates, Compose files and workflows. The
   **scanner** finds env var reads in source code. **gitutil** asks `git`
   which env files are tracked or ignored.
4. The **project model** combines everything. It is built once and never
   changes.
5. **Rules** (EG001 to EG016) are pure functions of the model. They run
   concurrently and produce findings.
6. Findings get their severity overrides, are suppressed (config `ignore`,
   inline directives), and are sorted by file, line, column and rule ID.
7. **Reporters** render the findings. Any value text goes through
   `internal/mask`.
8. The exit code is 0, 1 or 2 ([CI](ci.md#exit-codes-and-fail-on)). Only
   `cmd/envguard` maps errors to exit codes.

The same input gives byte-identical output.

## Discovery

Discovery walks the tree from `--root` and never fails because of one
unreadable file.

**Skipped directories:** `.git`, `node_modules`, `vendor`, `dist`, `build`,
`target`, `.venv`, `venv`, `__pycache__`, `.next`, `.nuxt`, `coverage`, plus
anything matched by [`exclude`](configuration.md#include-and-exclude).

**`.gitignore`:** all nested `.gitignore` files and `.git/info/exclude` are
honored with a pure-Go matcher (no `git` needed). Ignored directories are not
entered. Ignored files are skipped, **except env files and templates**: they
are kept and flagged as ignored, because rules need the content of `.env`, and
EG009 needs to know whether it is ignored.

**Symlinks:**

| Case | Behavior |
|---|---|
| Resolves outside the root | Skipped. |
| Points to its own directory or an ancestor, or loops | Skipped. |
| Directory link inside the root | Not followed; its content is reached through its real path. |
| File link inside the root | Followed, reported under the link path. |

Devices and FIFOs are never opened.

**Limits:** depth 25, 50,000 files (the walk then stops and a warning says
results are incomplete), 1 MiB per source, Compose or workflow file (larger
ones are skipped). Env files are kept regardless of size, and the parser
reports a file over 1 MiB as a problem. A file with a NUL byte in its first
8 KiB is treated as binary and skipped.

### File kinds

| Kind | Rule |
|---|---|
| Env files and templates | Names beginning with `.env`, see below. |
| Compose | `docker-compose` or `compose`, optionally followed by `.`, `-` or `_` and more, with `.yml` or `.yaml`, in any directory. `composer.yml` is not Compose. |
| Workflows | `.yml` or `.yaml` directly in `.github/workflows/` at the root. |
| Source | By extension: `.js .jsx .mjs .cjs .ts .tsx .vue .svelte .py .go .rb .php .rs .java .kt .cs .ex .exs .sh`. |

### Environment names

| File | Kind | Environment |
|---|---|---|
| `.env` | real | `default` |
| `.env.local` | real, local | `development` |
| `.env.production` | real | `production` |
| `.env.production.local` | real, local | `production` |
| `.env.example` | template | `default` |
| `.env.production.example` | template | `production` |
| `.env.staging.sample` | template | `staging` |

The last suffix `example`, `sample`, `template`, `dist` or `defaults` makes a
file a template. Names are normalized with these aliases; anything else is used
as written, lower-cased:

| Written | Becomes |
|---|---|
| `dev`, `development`, `local` | `development` |
| `test`, `testing` | `test` |
| `stage`, `staging` | `staging` |
| `prod`, `production` | `production` |

[`environments`](configuration.md#environments) in the config overrides this
inference.

### App roots

A directory with a marker file is an app root: `package.json`, `go.mod`,
`pyproject.toml`, `requirements.txt`, `setup.py`, `Pipfile`, `Gemfile`,
`composer.json`, `Cargo.toml`, `pom.xml`, `build.gradle(.kts)`, `mix.exs`,
`deno.json(c)` or `*.csproj`. The repository root is always an app root. Every
file belongs to its nearest app root. Rules use app roots to match templates,
env files and code in monorepos. Dockerfiles and Compose files are
deliberately not markers.

## The dotenv parser

The parser keeps exact positions (1-based line and column, columns counted in
Unicode code points) and handles LF, CRLF, lone CR and a UTF-8 BOM. It never
panics and runs in linear time.

- Keys use ASCII letters, digits, `_`, `.` and `-`, and cannot start with a
  digit. `export KEY=v` is accepted.
- Unquoted values end at a `#` that follows a space or tab. A `#` inside a
  word is part of the value.
- Single quotes are literal and single-line. Double quotes allow the escapes
  `\n \t \r \" \\ \$` and may span lines. Backticks are literal and may span
  lines.
- References `$VAR`, `${VAR}` and `${VAR:-default}` are recorded, never
  expanded.
- Syntax problems (invalid key, missing `=`, unterminated quote) are reported
  as EG007. Problem messages are fixed text, because a malformed "key" may
  itself be a secret.
- Duplicate keys are kept, in order (EG006).

## The source scanner

The scanner finds reads of environment variables in source code. Its design
rule is **precision over recall: a missed usage is acceptable, a phantom usage
is not.**

### A lexer, not just regexes

Each language has a small lexer that marks every byte as code, comment or
string. A match counts only if it **starts in code**. So these are all ignored:

```js
// process.env.IN_A_COMMENT
const doc = "process.env.IN_A_STRING";
/* import.meta.env.IN_A_BLOCK_COMMENT */
```

The variable name inside `os.Getenv("X")` is fine, because only the start of
the match is checked. The lexer understands JS template literals (code inside
`${...}`) and regex literals, Rust raw strings, lifetimes and nested comments,
Ruby `=begin`/`=end`, C# verbatim strings, Python docstrings and shell
quoting. In Vue and Svelte files only `<script>` bodies count, and in PHP only
`<?php` and `<?=` regions. Matches that are members of something else
(`foo.process.env`, `myos.Getenv`, `$this->env()`) are rejected.

### Supported patterns

Dot-access patterns accept only `UPPER_CASE` names, so `process.env.hasOwnProperty`
is ignored.

| Language | Patterns |
|---|---|
| JavaScript, TypeScript (`.js .jsx .mjs .cjs .ts .tsx`) | `process.env.X`, `process.env["X"]`, `import.meta.env.X`, `import.meta.env["X"]`, `Deno.env.get("X")`, `Bun.env.X`, `Bun.env["X"]`, destructuring `const { A, B = "d" } = process.env` (also `import.meta.env`, `Bun.env`) |
| Vue, Svelte | The same patterns, inside `<script>` only |
| Python | `os.environ["X"]`, `os.environ.get("X"[, d])`, `os.getenv("X"[, d])`; `environ["X"]` and `environ.get(...)` when the file has `from os import ... environ`; django-environ `env("X")`, `env.str("X")`, `env.int(...)` and similar when the file imports `environ`, or when the app's dependency files (`requirements*.txt`, `requirements/*.txt`, `pyproject.toml`, `Pipfile`, `setup.py`, `setup.cfg`) name `django-environ` (settings are usually split over modules and only one imports it) |
| Go | `os.Getenv("X")`, `os.LookupEnv("X")`, struct tags `env:"X"` with optional `envDefault:"..."` (caarlos0/env style). In a file that uses `envPrefix`, the real names depend on how structs nest, so every tag name is recorded as an *inferred* read, bare and combined with each prefix of the file |
| Ruby | `ENV["X"]`, `ENV.fetch("X"[, d])`, `ENV.fetch("X") { ... }` |
| PHP | `getenv('X')`, `$_ENV['X']`, `$_SERVER['X']` (request metadata such as `HTTP_*` and `REQUEST_*` is ignored), Laravel `env('X'[, d])` |
| Rust | `env::var("X")`, `std::env::var_os("X")`, `env!("X")`, `option_env!("X")` |
| Java | `System.getenv("X")`, `System.getenv().getOrDefault("X", d)` |
| Kotlin | The Java patterns, plus the elvis default `System.getenv("X") ?: "d"` |
| C# | `Environment.GetEnvironmentVariable("X")` |
| Elixir | `System.get_env("X"[, d])`, `System.fetch_env!("X")` |
| Shell (`.sh`) | `${X}`, `$X`, `${X:-d}`, `${X-d}`, `${X:=d}`, `${X:+a}`, `${X:?msg}`, `${!X}` |

Shell details: only `UPPER_CASE` names count, and a script that is not a shell
script (for example a Python shebang) is skipped. Variables the script assigns
itself (`X=...`, `export X=...`, `read X`, `for X in`) are not environment
reads, except `X=${X:-d}`, which reads the environment. Shell built-ins such as
`RANDOM` and `BASH_*` are ignored, and single quotes are literal.

### How defaults are detected

A usage "has a default" (is *optional*) when the read tolerates absence. That
changes rule severities: for example EG003 is `info` instead of `error` when
every read is optional. A read counts as optional when a default follows it,
when its value is compared (the code branches on absence), or when the API
cannot fail.

| Language | Counts as a default |
|---|---|
| JS, TS | `|| x`, `?? x` after the read; a comparison `process.env.X === "true"`, `!==`, `==`, `!=`; destructuring `B = "d"` |
| Python | a second argument: `os.getenv("X", "d")`; a comparison or `in` test on `os.getenv(...)`/`os.environ.get(...)` (not on `os.environ["X"]`, which raises `KeyError`); django-environ `default=` |
| Go | `os.LookupEnv` always; `os.Getenv("X") == "1"` or `!= ""`; `envDefault:` in a struct tag |
| Ruby | `ENV.fetch("X", d)`, a fetch block, `ENV["X"] \|\| d`, `ENV["X"] \|\|= d`, `ENV["X"] == "1"` |
| PHP | `?:`, `??`, `\|\|` or a comparison after `getenv`; `??` after `$_ENV`/`$_SERVER`; Laravel `env('X')` always (it returns the default or null and never fails) |
| Rust | `.unwrap_or*`, `.ok()`, `.map_or*`, `.is_ok()`, `.is_err()`; `option_env!` always |
| Java | `getOrDefault`; Kotlin `?:`; `== x` / `!= x` |
| C# | `?? d` or a second argument |
| Elixir | `System.get_env("X", d)` or `\|\| d` |
| Shell | `${X:-d}`, `${X-d}`, `${X:=d}`, `${X:+a}`; `${X:?}` is required |

Assignments (`process.env.X = "..."`, `os.environ["X"] = ...`) are not reads
and are skipped.

### Dynamic lookups and whole-environment reads

A computed key such as `process.env[name]` or `os.Getenv(key)` is recorded as
a **dynamic usage**. EnvGuard cannot know the name, so it never produces a
finding for it. It does count them (`dynamic_lookups` in
[JSON output](json-output.md#stats), and a note in `doctor`), and EG004 mentions
them, because a "declared but unused" variable may be read dynamically. A rest
element in destructuring (`const { A, ...rest } = process.env`) is dynamic too.

JavaScript and TypeScript code that hands the **whole environment to a
function** (`schema.parse(process.env)`, `createEnv({ runtimeEnv: process.env })`,
`function load(source = process.env)`) is a *bulk read*: the variable names live
in the schema. EG004 then stays silent in the whole repository, because it
cannot know what is unused. Passing the environment to a child process
(`spawn(cmd, { env: { ...process.env } })`) is not a bulk read.

### Well-known variables

Variables provided by the OS, the shell or a hosting platform never trigger
"undeclared" findings: `PATH`, `HOME`, `USER`, `SHELL`, `PWD`, `TERM`, `LANG`,
`TZ`, `TMPDIR`, `CI`, `NODE_ENV`, `HOSTNAME`, `AWS_REGION`,
`AWS_DEFAULT_REGION`, the terminal conventions (`NO_COLOR`, `FORCE_COLOR`,
`COLORTERM`, ...), ambient session variables (`EDITOR`, `LOGNAME`, `DISPLAY`,
`APPDATA`, `USERPROFILE`, ...), proxy settings (`HTTP_PROXY`, `NO_PROXY`, ...),
`VERCEL`, `VITEST`, `JEST_WORKER_ID`, and names starting with `XDG_`, `LC_`,
`CI_`, `GITHUB_`, `RUNNER_`, `VERCEL_`, `NETLIFY_`, `RENDER_`, `FLY_`,
`HEROKU_`, `VITEST_` or `KUBERNETES_SERVICE_`. `PORT` counts only when the code
gives it a default. AWS credentials are deliberately not on the list. Add your own with
[`scanner.ignore_variables`](configuration.md#scanner).

### Framework notes

For JavaScript-family apps, EnvGuard reads the app root's `package.json` and
attaches informational notes (shown by `doctor` and in the JSON `notes` array).
They never produce findings.

- **Next.js** (`next` in dependencies): only `NEXT_PUBLIC_*` and `NODE_ENV` are
  inlined into browser code. A file is client code if it starts with a
  `"use client"` directive (within its first 512 bytes) or lives under a
  `pages/` directory but not `pages/api/`.
- **Vite** (`vite` in dependencies): only `VITE_*` (and `MODE`, `BASE_URL`,
  `PROD`, `DEV`, `SSR`) reach client code. Files under the app's `src/` are
  treated as client code.

If both are listed, Next.js wins.

## Compose and workflows

**Docker Compose.** For each service EnvGuard reads `environment` (map and
list forms, merge keys, pass-through entries like `- KEY`) and `env_file`
(string, list or long syntax with `required: false`; paths resolve against the
Compose file's directory). It also finds interpolations anywhere in the file:
`${V}`, `${V:-d}`, `${V-d}`, `${V:+a}`, `${V:?err}` and `$V`. `$$` is a
literal `$`. Compose reads `.env` next to the Compose file for interpolation,
and EnvGuard follows that rule. Values are never stored.

**GitHub Actions workflows.** `env:` keys at workflow, job and step level are
definitions. `secrets.X`, `secrets['X']`, `vars.X` and `env.X` inside
`${{ ... }}` and in `if:` conditions are references. `secrets.GITHUB_TOKEN` is
ignored. The result is the CI inventory in `doctor` and in the JSON `ci`
object.

Both parsers walk YAML without following aliases, so "billion laughs"
documents stay linear. A file that is not valid YAML is reported as EG007.

## Git checks

EnvGuard runs `git` only through fixed argument lists, with no shell and a 10
second timeout per call: `rev-parse --is-inside-work-tree`, `ls-files -z
--cached` and one batched `check-ignore`. It disables `core.fsmonitor`
(otherwise a repository's config could make `git` run a program) and uses
`GIT_OPTIONAL_LOCKS=0` so nothing is written.

This powers EG008 (tracked env file), EG009 (env file not ignored) and the
tracked-file part of EG010. If `git` is missing, the directory is not a
repository, or you use `--process-env`, those checks stay silent.
`check --verbose` logs the reason.

## Secret masking

All value text reaches output through `internal/mask`. A test
(`TestLeakCanary`) runs every command and format on a repository full of
unique canary values and fails if one appears anywhere.

`mask.Display` shows a value only if **all** of these hold:

- the name is not sensitive (it does not contain `SECRET`, `TOKEN`,
  `PASSWORD`, `PRIVATE`, `CREDENTIAL`, `API_KEY`, `WEBHOOK`, and similar, and
  is not a whole token like `KEY`, `PWD`, `AUTH`, `DSN` or `PASS`),
- the schema does not mark it `sensitive: true`,
- it has at most 24 characters on one line, without control or bidirectional
  characters,
- it has no URL userinfo and no known credential prefix (`sk_live_`, `ghp_`,
  `AKIA`, ...),
- it does not look random (16 or more characters mixing upper case, lower case
  and digits).

URLs show only `scheme://host/path`; userinfo, query and fragment are removed.
Everything else becomes `•••••• (N chars)`, and an empty value is `(empty)`.
A value that `LooksLikeSecret` flags is masked whatever its name.

EG010 uses `LooksLikeSecret` to detect these formats anywhere in a value:

| Kind | Examples |
|---|---|
| PEM private key | `-----BEGIN ... PRIVATE KEY-----` |
| AWS access key ID | `AKIA`, `ASIA`, `ABIA`, `ACCA` prefixes |
| GitHub token | `ghp_`, `github_pat_`, `gho_`, `ghu_`, `ghs_`, `ghr_` |
| Slack | `xox?-` tokens and webhook URLs |
| Stripe | `sk_live_`, `rk_live_` (test keys are not secrets) |
| Anthropic, OpenAI-style | `sk-ant-...`, `sk-...` |
| Google API key | `AIza...` |
| SendGrid | `SG....` |
| Twilio | `SK` followed by 32 hex characters |
| JWT | three base64url parts |
| URL with a password | `postgres://user:realpassword@host/db` |
| High-entropy value | 20 or more characters, no spaces, Shannon entropy of at least 3.5 bits per character, **only for sensitive names** |

Placeholders are not reported: `changeme`, `replace-me`, `your-...`, `<...>`,
`${...}`, `xxxx`, a value equal to its key, localhost URLs without credentials,
and similar. Passwords in URLs are ignored when the host is local, the password
equals the user name, or it is a placeholder. A finding names the kind
(`detected: AWS access key ID`), never the value.

## Known limitations

EnvGuard prefers to miss something over reporting something wrong. These are
the cases we know about; most of them only cause a missing finding or an `info`
note, and all of them can be silenced with
[`scanner.ignore_variables`](configuration.md#scanner) or an `ignore:` entry.

**Reads the scanner cannot see** (EG004 may call such a variable unused):

- **Dynamic keys.** `process.env[name]` and `os.Getenv(key)` are counted but
  not analyzed. EnvGuard cannot tell which variable they read.
- **Whole-environment reads** (`schema.parse(process.env)`) are recognized in
  JavaScript and TypeScript only. In other languages (`dict(os.environ)`,
  `ENV.to_h`) the names are not known and EG004 may report variables that the
  schema reads.
- **ERB and YAML.** Reads inside Rails `config/*.yml` files (`<%= ENV["X"] %>` in
  `database.yml`, `storage.yml`) are not scanned, because EnvGuard does not
  parse ERB. Variables used only there may be reported by EG004.
- **Reads by frameworks and libraries themselves** (`RAILS_MASTER_KEY`, Laravel's
  `BCRYPT_ROUNDS`, a Prisma `env("DATABASE_URL")` in `schema.prisma`) happen
  outside the scanned code.
- **String interpolation.** Python f-strings, and Ruby, Kotlin and C# string
  interpolation, are not scanned.
- **Settings models.** pydantic-settings field models are not understood.
- **Go `envPrefix`.** Names under a prefix are inferred, not resolved: EG004
  counts them as read, but EG003 cannot report an undeclared one, and a
  variable that matches a prefix by accident may be called read.
- **Heredocs.** The body of a shell heredoc is treated as code.
- **Extensionless scripts.** Only the listed extensions are scanned.
- **Other languages.** Languages not in the table are not scanned. If a whole
  app root has no scanner usages, EG004 skips it, and `check --verbose` says
  so.

**Judgement calls**

- **Optional reads.** EG003 is `error` only for a read that does not look
  optional. `process.env.X` followed by nothing is treated as required, even
  when the app copes without it; use `# optional` in the template or an ignore.
  A repository that declares no variable at all gets `info` suggestions, never
  errors.
- **Local `.env` files.** EG001 compares a real `.env` with its template and
  reports every template variable the file lacks, including ones the code
  defaults. Mark such lines `# optional` in the template.
- **Unquoted values with spaces** (`FROM=Shop <a@b.c>`) are an EG007 warning:
  dotenv libraries accept them, but shells and some loaders (PHP's phpdotenv)
  do not.
- **`${VAR}` without a default in Compose** (EG011) is reported even when a
  wrapper script exports it (Laravel Sail's `WWWUSER`): Compose itself warns.
- **Monorepo heuristics.** App roots come from marker files. EG003 treats a
  variable as declared if any env file, template, schema entry, Compose
  definition or workflow in the repository declares it. That avoids noise but
  can hide a variable declared for a different app.
- **Framework notes** are heuristics from `package.json` and file locations.

**Scope**

- **No git history.** EnvGuard checks the current tree and which env files are
  tracked. It does not scan commit history for leaked secrets. Use a tool such
  as [gitleaks](https://github.com/gitleaks/gitleaks) for that, and rotate any
  secret that was ever committed.
- **Secret detection** matches known formats and entropy for sensitive names.
  It is not a complete secret scanner.
- **EG004 and the schema.** "Declared but unused" is not yet checked for
  variables that appear only in `.envguard.yaml`, because they have no position
  in an env file.
- **Limits.** Walks stop at 50,000 files, sources over 1 MiB are skipped, and
  directories deeper than 25 levels are not entered.
- **Huge names.** A variable name of hundreds of kilobytes is analyzed, but
  table columns clip names to 48 characters; JSON keeps the full name.

## Hostile repositories

EnvGuard is meant to be run on code you have just cloned, so it treats the
repository as untrusted:

- File names, directory names and YAML keys are shown with control characters
  (terminal escape sequences, bidirectional overrides, invalid UTF-8) replaced
  by visible escapes such as `\x1b`, in every text format, log line and error.
  JSON and SARIF keep the raw names; their encoders escape them.
- Symlinks that leave the repository are not followed, link loops and
  non-regular files (FIFOs, devices) are never opened, and `env_file` paths
  that point outside the repository are neither read nor reported.
- Binary, oversized and deeply nested files are skipped, and invalid UTF-8
  never causes a crash.
- `git` runs with fixed arguments, no shell, and with the repository's
  `core.fsmonitor` disabled, so a malicious git config cannot run a program.
- A panic in a rule or a scanner worker becomes an error or a skipped file,
  never a crash dump, and never prints a value.

# CLAUDE.md — EnvGuard

This file is the permanent memory for every session working on this repository.
Read it fully before making changes. Keep it accurate: if you change the
architecture, a rule, an exit code or a convention, update this file in the
same change.

---

## 1. Vision

> **"Your code knows which env vars it needs. EnvGuard makes sure your config agrees — before you deploy."**

EnvGuard is a single-binary, open-source CLI written in Go that catches
environment-variable and configuration mistakes before an app runs or deploys.

- **One command**: `envguard check` in any repository.
- **Zero config required**: sensible defaults; no setup file needed.
- **Explainable findings**: every finding says what is wrong, why it matters, and how to fix it.
- **Safe by default**: offline, read-only, never prints secret values.

EnvGuard cross-checks these sources against each other:

| Source | Examples |
|---|---|
| Source code usage | `os.Getenv("X")`, `process.env.X`, `os.environ["X"]`, `ENV["X"]`, … |
| Real env files | `.env`, `.env.local`, `.env.production`, … |
| Env templates | `.env.example`, `.env.sample`, `.env.template`, `.env.production.example`, … |
| Docker Compose files | `docker-compose.yml`, `compose.yaml`, `compose.*.yaml`, … |
| GitHub Actions workflows | `.github/workflows/*.yml` / `*.yaml` |
| Optional schema | `.envguard.yaml` |

### CLI surface

| Command | Purpose |
|---|---|
| `envguard check` | Run all rules, print findings, exit per `--fail-on`. `--process-env` validates the process environment instead of env files; `--app` picks an app root. |
| `envguard doctor` | Human-friendly health report of the repo's env setup. Exit 0 unless `--fail-on` is given. **`envguard` without a command runs doctor.** |
| `envguard diff [envA envB]` | Without args: variables × environments matrix per app root. With two envs: only the differences. `--exit-code` → 1 on differences. |
| `envguard init` | Generate a starter `.envguard.yaml` (and `.env.example` with `--example`) from templates, code, Compose and workflows. Refuses to overwrite without `--force`; `--dry-run` prints. Never copies values. |
| `envguard explain <RULE>` | Print the full documentation for a rule (from embedded `docs/rules/*.md`); accepts `EG001`, `eg1` or a rule name. `--list` (or no arg) lists all rules. |
| `envguard version` | Print version, commit and build date (injected via `-ldflags`). |

Global flags: `--root` (default `.`), `--config`, `--no-color`, `--ascii`, `--verbose`.
`check` and `doctor` (and the root command) take `--format
pretty|doctor|json|sarif|github` and `--output FILE`; `diff` takes `--format
pretty|json` and `--output`.

---

## 2. Non-negotiable principles

These are hard requirements. A change that violates one of them is a bug,
regardless of what else it achieves.

1. **Offline only.** No network calls, no telemetry, no update checks, ever.
   EnvGuard's own packages must never import `net` or `net/http`, and no
   network client (`net/http*`, `net/rpc*`, `net/smtp`, `net/mail`,
   `crypto/tls`) may appear anywhere in the dependency graph. Enforced by
   `TestOffline` (`cmd/envguard/offline_test.go`, runs `go list -deps`).
   Known exception: `github.com/spf13/pflag` (required by cobra) imports
   `net` only for IP-address flag parsing. On Windows only,
   `golang.org/x/sys/windows` (syscall bindings used by termenv/x/term for
   console modes) also imports `net` for socket-related types; EnvGuard calls
   only its console functions. Those two are the entries of `netAllowlist`
   (the first CI run on Windows found the second). Any new third-party package
   importing `net` fails the test until reviewed and justified there.
2. **Read-only.** EnvGuard never modifies user files. The exceptions are
   `envguard init`, which writes `.envguard.yaml` (and `.env.example` with
   `--example`), refuses to overwrite existing files unless `--force` is
   given, and never copies a value from any env file into what it writes;
   `--output FILE`, which writes the report where the user asked (creating
   parent directories); and the github format, which appends a job summary to
   `$GITHUB_STEP_SUMMARY` when that variable is set.
3. **Never print secret values.** Every output path — terminal, JSON, SARIF,
   GitHub annotations, error messages, verbose/debug logs — receives value text
   **only** through `internal/mask`. No other package may format a raw value
   into output. Wrapped errors must not carry raw values either (e.g. parser
   errors report positions and keys, not values). **Enforced forever by
   `TestLeakCanary` (`cmd/envguard/leak_canary_test.go`)**: a repo where every
   value is a unique generated canary containing `CNRY` is run through every
   command, flag combination, error path and the raw findings JSON, and no
   output may contain `CNRY`. Every new output path (reporter, command, log
   line, error) must be added to that test; never weaken or delete it.
4. **Zero config works.** `.envguard.yaml` is optional and only adds power.
   Every rule must behave sensibly without it.
5. **Every finding explains WHY it matters and HOW to fix it.** No finding is
   emitted with an empty `Why` or `Fix`.
6. **Deterministic output.** Findings are stable-sorted by file, line, column,
   rule ID (then variable, then message as tie-breakers). Map iteration order
   must never leak into output. Same input ⇒ byte-identical output. The one
   exception: the run duration in the pretty summary, which is printed only
   when stdout is a terminal (`report.Options.ShowDuration`); `--verbose`
   timings go to stderr.
7. **Low false positives beat more rules.** When unsure, use severity `info`
   or don't emit. A noisy rule is worse than a missing one.

---

## 3. Architecture

Module path: `github.com/ardamoustafa1/envguard` (placeholder — replace with
the real GitHub owner everywhere when the repo is published).

```
cmd/envguard        main; the ONLY place that maps errors to exit codes
internal/config     load + validate .envguard.yaml; merge flags > config > defaults
internal/discovery  walk the repo safely, classify files
internal/parser     dotenv parser with exact line/column positions
internal/scanner    detect env var usage in source code (many languages)
internal/compose    Docker Compose parsing
internal/workflow   GitHub Actions workflow parsing
internal/gitutil    tracked/ignored status via the git binary (optional)
internal/yamlutil   YAML helpers for compose/workflow: positions, merge keys, alias-safe walk
internal/schema     variable types and validation
internal/generate   files written by `envguard init` (pure: Project → bytes)
internal/project    builds ONE immutable Project model from all inputs
internal/rules      Rule interface, registry, all rules (pure functions of Project)
internal/findings   Finding model, severity, sorting, suppression
internal/mask       the ONLY place that decides what value text may be shown
internal/report     output formats: pretty (check), doctor, json, sarif, github;
                    environment matrix + diff; explain rendering
internal/testutil   test-only helpers (golden files, -update, CopyFixture, FakeSecret, Canary)
docs                package `docs`: embeds docs/rules/*.md via go:embed and parses
                    them (RuleDoc); docs/envguard.schema.json is generated by
                    internal/config; docs/json-output.md documents --format json
```

### Package responsibilities

- **cmd/envguard** — Holds the cobra command tree (`root.go`, one file per
  subcommand). `main()` executes it and converts the returned error into an
  exit code in exactly one function (`exitCode`). Nothing else calls
  `os.Exit`. Commands use `RunE` and `SilenceErrors`/`SilenceUsage`; `main`
  prints the error to stderr once. `run` wraps everything in `runGuarded`:
  a panic becomes one line (`internal error: …; this is a bug, please report
  it`, exit 2; the panic value is shown only for `runtime.Error`, never for
  values that could carry data).
  - Error messages tell the user what to do and never echo a value from the
    repository: `noArgs` (unknown command with a "did you mean", extra
    arguments), a flag-error hook that points at `--help`, `--env` values that
    name no environment of the repository are an **error** (a typo must not be a
    silent all-clear; `checkEnvironments`, by position), every command has
    `Example:` text (`TestEveryCommandHasExamples`).
  - `analyze.go`: the shared pipeline (config → discovery → project →
    rules) for check/doctor/diff, with `--verbose` logs and per-phase timings
    (`timing: discover 1.2ms`) on stderr; `failed(failOn, findings)`.
  - `output.go`: `--format` resolution (check auto-selects `github` when
    `GITHUB_ACTIONS=true` and `--format` is unset; unknown formats are exit 2
    and never echoed), terminal detection (`detectTerminal`: colors only on a
    TTY without `--no-color`/`NO_COLOR`/`TERM=dumb`, Windows VT processing via
    termenv; ASCII symbols with `--ascii`, TERM=dumb, a non-UTF-8 locale or a
    legacy Windows console; width from the TTY), `--output` (no colors), and
    the `$GITHUB_STEP_SUMMARY` job summary. Path errors are stripped of the
    path (`pathErr`).
  - Doctor ignores the config's `fail_on` (only `--fail-on` makes it fail).
    `diff` builds the project without running rules. `explain` needs no
    config. Errors never echo user arguments (--env, --app, --format, explain
    IDs, diff environments).
- **internal/config** — (implemented) `Load(Flags) (*Settings, error)`.
  - File lookup: `--config` (must exist) else `<root>/.envguard.yaml` or
    `.envguard.yml`; both present is an error; none is fine (defaults).
  - Shape: `config.File` — `version` (required, must be 1), `fail_on`,
    `include`, `exclude`, `environments` (name → `{files, templates}` globs),
    `rules` (`EGxxx` → error|warning|info|off), `ignore` (list of
    `{rule, path, variable, reason}`; needs path and/or variable), `scanner`
    (`languages`, `ignore_variables`), `profiles` (`production-safety`),
    `variables` (the schema: `description`, `required` (default false),
    `type`, `environments` (where `required` applies; default all; only
    allowed with `required: true`), `sensitive`, `default` (must satisfy the
    type), `values` (enum only), `schemes` (url only), `pattern`, `min`/`max`
    (int, float, port only), `min_length`/`max_length`, `production:
    {must_be, must_not_be, pattern}`). `fail_on` is an addition beyond the
    original field list so `--fail-on` has a config counterpart.
    `Variable.Spec()` converts to `schema.Spec`; `Variable.RequiredIn(env)`;
    `File.Positions` / `Settings.VariablePositions` give each variable's key
    position (not part of the YAML shape, `yaml:"-"`).
  - Schema validation is strict (exit 2, `file:line:col`): unknown type (with
    suggestion), enum without values, values/schemes/min/max where they do not
    apply, min > max, negative or inverted lengths, invalid regex (pattern and
    production.pattern), default or must_be/must_not_be invalid for the type,
    must_be == must_not_be, empty production rule, invalid environment names.
  - Strict decoding: the YAML node tree is walked against the structs via
    reflection: unknown keys, duplicate keys, wrong node kinds and wrong scalar
    types are reported as `file:line:col: message` with Levenshtein "did you
    mean" suggestions. Unknown keys don't stop value validation, so all
    problems are reported at once, in file order. Errors are `*config.Error`
    and match `config.ErrInvalid` → exit 2. Messages contain key names, never
    values (`TestErrorsDoNotEchoValues`).
  - Precedence: `Merge(Defaults(), file, flags)` — flags > file > defaults.
    A flag overrides only when set (`Flags` uses pointers/nil slices; cobra
    `Changed`). Slices/maps are copied, so `Settings` is immutable.
  - JSON Schema: `config.JSONSchema()` generates `docs/envguard.schema.json`
    from struct tags (`yaml`, `desc`, `enum`, `keys`, `pattern`, `required`,
    `const`). Every field needs a `desc`. Regenerate with
    `go test ./internal/config -run TestJSONSchemaInSync -update`.
  - Glob semantics (include/exclude/environments/ignore.path): doublestar,
    relative to root, forward slashes; a pattern **without `/` matches the
    base name at any depth**. Globs with a `..` segment, an absolute path or a
    drive letter are rejected ("must stay inside the root"). Command-line
    value errors use `usageError` (matches `ErrInvalid`, no suffix).
- **internal/discovery** — (implemented) `Discover(Options) (*Inventory, error)`;
  `Settings.DiscoveryOptions()` builds the options.
  - Skips: built-in dirs (`.git`, `node_modules`, `vendor`, `dist`, `build`,
    `target`, `.venv`, `venv`, `__pycache__`, `.next`, `.nuxt`, `coverage`),
    user excludes, `.gitignore` (all nested ones, plus `.git/info/exclude`;
    go-git's pure-Go matcher). **Gitignored directories are pruned; gitignored
    files are skipped except env files/templates**, which are kept with
    `GitIgnored: true` (rules need `.env` content; EG009 needs the flag).
  - Safety: symlinks resolving outside root → `symlink-outside-root`;
    links to an ancestor/self or link cycles → `symlink-loop`; directory
    symlinks inside root are **not followed** (`symlink-duplicate`, content is
    reached via its real path); file symlinks inside root are followed and
    reported under the link path. Non-regular files (FIFOs, devices) are never
    opened. Max depth 25, max 50,000 files (then `Truncated`), sources /
    compose / workflows > 1 MiB → `too-large` (env files are kept; the parser
    reports size), NUL in first 8 KiB → `binary`. Unreadable dirs/files are
    recorded, never fatal. `Skipped` only lists dirs and would-be-classified
    files.
  - Env names (`ClassifyEnvName`): `.env` → default; `.env.<x>` → x;
    `.env.<x>.local` → x (Local); `.env.local` → development (Local);
    last suffix `example|sample|template|dist|defaults` → template of the
    rest (`.env.example` → default). Aliases: dev/development/local →
    development, test/testing → test, stage/staging → staging,
    prod/production → production; else literal, lower-cased. `environments`
    overrides from config win over inference.
  - Other kinds: Compose = `docker-compose` or `compose` optionally followed by
    `.`/`-`/`_`, `.yml`/`.yaml`, any directory (`composer.yml` is not);
    workflows = `.github/workflows/*.yml|yaml` at the root only; sources by
    extension (see `extLanguages`).
  - App roots: nearest ancestor with a marker (`package.json`, `go.mod`,
    `pyproject.toml`, `requirements.txt`, `setup.py`, `Pipfile`, `Gemfile`,
    `composer.json`, `Cargo.toml`, `pom.xml`, `build.gradle[.kts]`, `mix.exs`,
    `deno.json[c]`, `*.csproj`); root is always `.`. Dockerfiles and Compose
    files are deliberately not markers. Every file gets an `AppRoot`.
  - Output sorted by path; paths relative to root, slash-separated.
- **internal/parser** — Dotenv parser (implemented). `Parse(r, filename)`
  returns `*File{Entries, Problems, Directives, FileIgnoreRules}`; the error
  is for I/O only, and it never panics. Decided semantics (change only
  deliberately, with tests and goldens):
  - Positions: `Pos{Line, Column, Offset}`; 1-based line/column, column counts
    Unicode code points (tab = 1), `Offset` is the byte offset in the original
    input including a BOM. `ValuePos` is the opening quote for quoted values,
    or right after `=` for empty values. Line endings: CRLF, LF, lone CR.
  - Keys: ASCII letters, digits, `_ . -`; not starting with a digit. Invalid
    key, missing `=`, empty key and unterminated quotes produce a Problem and
    **no entry**. Invalid escapes, trailing text after a closing quote and
    unquoted whitespace produce a Problem **and** keep the entry.
  - `export` is a prefix only if followed by blanks and then a key (`export=1`
    is key `export`). A bare `export KEY` is a missing-`=` problem.
  - Unquoted: inline `#` comment only when preceded by a space/tab (a `#`
    directly after `=` or inside a word is part of the value); trailing blanks
    trimmed; whitespace outside `${...}` is a warning. No escape processing.
  - Single quotes: literal, single-line only (no escapes; `\'` does not escape).
  - Double quotes: escapes `\n \t \r \" \\ \$`; anything else (including
    backslash-newline) is an invalid-escape error and is kept literally;
    multi-line allowed. Backticks: literal, multi-line. Newlines inside
    multi-line values are normalized to `\n` in `Value`; `RawValue` keeps bytes.
  - After a closing quote, `#` starts a comment even without preceding space.
  - Unterminated quote: error at the opening quote, parsing resumes at the
    line after it.
  - References (unquoted and double-quoted only): `$VAR`, `${VAR}`,
    `${VAR<op>word}` with op in `:- - := = :? ? :+ +`; `HasDefault` only for
    `-`/`=` forms. `$$` and `\$` are literal; refs nested inside `word` are not
    reported; unclosed `${` is ignored. References are never expanded.
  - Directives: `# envguard-ignore EG002,EG004` applies to the next assignment
    line (even a malformed one, so EG007 can be suppressed); stacked
    directives merge; inline `KEY=v # envguard-ignore EG005` applies to that
    entry; `# envguard-ignore-file EG002` applies to the file. IDs are
    case-insensitive (`EG` + 3 digits), separated by commas/spaces; text after
    `--` is a free-form reason. Bad/missing IDs are warning-level Problems.
    Use `File.IsIgnored(ruleID, line)`.
  - Duplicates are kept in order. Files > `MaxFileSize` (1 MiB) yield one
    `file-too-large` problem and no entries.
  - Problem `Message`/`Hint` are fixed strings and never contain input text
    (a malformed "key" may itself be a secret); `Key` is set only for valid
    keys. Guarded by `TestProblemsNeverEchoInput`.
  - Linear time on adversarial input (brace matching is one pass per value);
    guarded by `TestParsePathologicalInputs`.
- **internal/scanner** — (implemented) `Scan(Options{Root, Files})` →
  `Result{Usages, Scanned, Unreadable}`. **Precision over recall: a missed
  usage is acceptable, a phantom usage is not.**
  - `Usage{Variable, File, Line, Column, Language, HasDefault, Dynamic,
    Pattern, AppRoot, Note, Bulk, Inferred}`; Line/Column point at the name
    (or the expression start for Dynamic). Sorted by file/line/col/variable.
    `HasDefault` really means "optional": a default follows, the value is
    compared (`process.env.X === "true"`, `os.Getenv("X") == "1"`,
    `os.getenv("X") in (...)`; not `os.environ["X"]`, which raises), or the
    API cannot fail (**Laravel `env()` is `alwaysDefault`**). `Bulk` marks a
    Dynamic usage that hands the *whole* environment to a function
    (`schema.parse(process.env)`, `runtimeEnv: process.env`,
    `source = process.env`; a spread or `env: process.env` for a child
    process is not bulk). `Inferred` marks Go names under `envPrefix`. Use
    `u.Certain()` (not Dynamic, not Inferred, has a name) wherever a usage is
    evidence that a variable exists (EG001 scoping, EG003, init, stats).
  - **Libraries** (`libraries.go`): per app root, the Python dependency files
    (`requirements*.txt`, `requirements/*.txt`, `pyproject.toml`, `Pipfile`,
    `setup.py`, `setup.cfg`) are searched for `django-environ`; a detector
    with `appLib` then runs in modules that do not import `environ`
    themselves (settings are split over base/production modules).
  - Detector regexes **must not start with `\b`**: it defeats Go's literal-prefix
    search (≈3x slower on large repos). `prevOK` already rejects matches
    glued to a longer name (`TestMatchesGluedToIdentifiersAreRejected`). A
    detector may have `before` (regexp anchored at the end of the preceding
    40 bytes, a stand-in for lookbehind).
  - A panic while scanning one file (`scanOne`) skips that file (it is listed
    in `Unreadable`); it never takes the run down.
  - Design: one `syntax` (lexer description) + compiled `detector` regexes per
    language (`detectors.go`). The lexer (`lexer.go`) classifies every byte as
    code/comment/string; **a match counts only if it starts in code**, so
    usages inside comments, docstrings, strings, regex literals and HTML are
    ignored (the name inside `os.Getenv("X")` is fine: only the start is
    checked). Special handling: JS template `${…}` is code (nested), JS regex
    literals, Rust raw strings and lifetimes vs char literals, nested Rust
    comments, Ruby `=begin/=end`, C# verbatim strings, shell `"…"`/`` `…` `` are
    code but consumed as a unit (so an apostrophe inside them never opens a
    string), Vue/Svelte only `<script>` bodies, PHP only `<?php`/`<?=` regions.
    `prevOK` rejects matches that are members of something else
    (`foo.process.env`, `myos.Getenv`, `$this->env()`).
  - Detector fields: `hint` (literal pre-filter), `requires` (file-level gate,
    e.g. `from os import environ`, `import environ`), `after` (default follows:
    `||`, `??`, `?:`, `.unwrap_or*`, `.ok()`, Ruby fetch block), `skipAfter`
    (assignments `X = …`, method calls), `alwaysDefault` (LookupEnv,
    option_env!, getOrDefault), `upperOnly` (dot-access patterns only accept
    UPPER_CASE, so `process.env.hasOwnProperty` is ignored), `skipNames` /
    `skipPrefixes` (Vite built-ins, `$_SERVER` request keys like `HTTP_*`).
    Custom extractors: JS destructuring (`{ A, B = "d", ...rest }` — rest is
    Dynamic), Go `env:"X"` struct tags in raw strings (`envDefault` = default;
    files using `envPrefix` report every tag name as an *inferred* usage, bare
    and with each prefix, because prefixes change names), shell.
  - Shell (`.sh`, skipped if the shebang is not a shell): `${X}`, `$X`,
    `${X:-d}`/`${X-d}`/`${X:=d}`/`${X:+a}` = default, `${X:?}` = required,
    `${!X}` = Dynamic. Only UPPER_CASE names; variables the script assigns
    (`X=…`, `export X=…`, `read X`, `for X in`) are not env reads, except
    `X=${X:-d}` (an env read with a default); shell built-ins (`RANDOM`,
    `BASH_*`, …) ignored; single quotes are literal.
  - Not supported on purpose (precision): Python f-strings, Ruby/Kotlin/C#
    string interpolation, pydantic-settings field models, envconfig prefixes,
    heredoc bodies (treated as code), extensionless scripts.
  - Framework notes (`framework.go`, heuristic, info only, `Usage.Note`):
    from the app root's package.json — Next.js (client file = `"use client"`
    directive in the first 512 bytes, or under a `pages/` dir but not
    `pages/api/`; public = `NEXT_PUBLIC_*`, `NODE_ENV`) and Vite (client file =
    under the app's `src/`; public = `VITE_*`). Next wins if both are listed.
  - `IsWellKnown(name, hasDefault)`: PATH, HOME, USER, SHELL, PWD, TERM, LANG,
    TZ, TMPDIR, CI, NODE_ENV, HOSTNAME, AWS_REGION, AWS_DEFAULT_REGION, the
    terminal conventions (NO_COLOR, FORCE_COLOR, COLORTERM, ...), ambient
    session variables (EDITOR, LOGNAME, DISPLAY, APPDATA, USERPROFILE, ...),
    proxy settings, platform/test-runner markers (VERCEL, VITEST,
    JEST_WORKER_ID, DYNO, ...), prefixes XDG_, LC_, CI_, GITHUB_, RUNNER_,
    VERCEL_, NETLIFY_, RENDER_, FLY_, HEROKU_, VITEST_, KUBERNETES_SERVICE_;
    PORT only with a default. AWS credentials are deliberately not on the
    list.
  - Performance: worker pool of GOMAXPROCS, each file read once, regexes
    compiled once at init, `hint` pre-filter. Baseline (Apple M4, 10 cores):
    `BenchmarkScan10kFiles` ≈ 213 ms for 10,000 files / 26 MB,
    `BenchmarkDiscoverAndScan10kFiles` ≈ 371 ms. Keep it well under 1 s.
    End to end (`cmd/envguard/bench_test.go`, generated monorepo: 40 apps,
    20,000 source files, 200 env files, git): `check` ≈ 1.25 s, `doctor` ≈
    1.3 s, `diff` ≈ 0.35 s (it builds the project with `EnvFilesOnly`),
    `envguard version` ≈ 3.5 ms; the raw open+read floor of the 20k files on
    that machine is ≈ 0.6 s. Remaining known cost: discovery opens every
    source file once to detect binaries and the scanner opens it again.
    Profile with `go test ./cmd/envguard -run '^$' -bench Monorepo -cpuprofile cpu.out`.
  - Tests: per-detector tables (`scanner_test.go`); fixture projects per
    language in `internal/scanner/testdata/scanner/` where every `FAKE_*` name
    must never be reported and every `REAL_*` token must be (except
    `REAL_DYNAMIC`); golden `testdata/scanner.golden.json`.
- **internal/yamlutil** — (implemented) shared by compose/workflow. `Parse`
  (node tree; syntax errors become a `Problem` whose message has quoted
  content replaced by "value"), `Pairs`/`Get` (mapping access expanding `<<`
  merge keys one alias level; **keys that are not scalars are skipped**, found
  by fuzzing), `WalkScalars` (keys and values, **never follows
  aliases** → billion-laughs documents stay linear), `Locator`/`Cursor` (find
  a token from a decoded scalar in the raw source → exact line and character
  column; falls back to the node position).
- **internal/compose** — (implemented) `Parse(data, rel) *File{Path, Dir,
  Services, Interpolations, Problem}`. Per service: `environment` (map and list
  forms; `PassThrough` for `KEY`/`KEY:` without value; merge keys honored),
  `env_file` (string, list, long syntax with `required: false`; paths resolved
  against the Compose file's dir; absolute/`${…}`/outside-root paths resolve to
  "" and are not checked; `Exists` is filled by project.Build). Interpolations
  everywhere in the file: `${V}`, `${V:-d}`/`${V-d}`/`${V:+a}`/`${V+a}`
  (HasDefault), `${V:?e}`/`${V?e}` (Required), `$V`; `$$` is a literal `$`;
  nested `${…}` inside a default are not reported. Position = the variable
  name. **Compose reads `Dir/.env` for interpolation** (`AdjacentEnv()`).
  Values are never stored.
- **internal/workflow** — (implemented) `Parse(data, rel) *File{Path, Refs,
  Problem}`. `Ref{Name, Context secrets|vars|env, Define, Level
  workflow|job|step, Job, Line, Column}`: `env:` keys at workflow, job and step
  level are definitions; `secrets.X`, `secrets['X']`, `vars.X`, `env.X` inside
  `${{ … }}` and in `if:` values (implicit expressions) are references.
  Accesses preceded by `.` (e.g. `steps.x.outputs.env`) are not contexts.
  `secrets.GITHUB_TOKEN` is ignored. Values are never stored.
- **internal/gitutil** — (implemented) `Inspect(ctx, root, paths) *Status{
  Available, Reason, Tracked, Ignored}` via `git -C root …` with **fixed
  argument lists, no shell, 10 s timeout per call**: `rev-parse
  --is-inside-work-tree`, `ls-files -z --cached` (tracked, relative to root),
  one batched `check-ignore -z --stdin` (exit 1 = nothing ignored, not an
  error). Safety: `-c core.fsmonitor=false` (a repository's fsmonitor setting
  would otherwise make even `ls-files` execute a program from that repo —
  verified, guarded by `TestInspectIgnoresMaliciousFsmonitor`),
  `core.untrackedCache=false`, `GIT_OPTIONAL_LOCKS=0` (read-only),
  `GIT_TERMINAL_PROMPT=0`, stderr discarded. Git missing / not a repo /
  failure → `Available=false` with a Reason; never an error. Rules needing git
  (EG008, EG009, the tracked-file part of EG010) then stay silent and
  `check --verbose` logs the reason.
- **internal/schema** — (implemented; does not import config) `Spec{Type,
  Values, Schemes, Pattern, Min, Max, MinLength, MaxLength}`, `Types()`,
  `ValidateSpec(Spec) []SpecError{Field, Message}`, `Spec.Check(value)
  (reason, ok)` — empty values always pass (that is EG005's job); reasons
  describe what was expected and never contain the value — `Spec.Describe()`
  ("an integer between 1 and 64"), `Spec.Equal(a, b)` (typed: off == false,
  007 == 7), `ParseBool` (true/false, 1/0, yes/no, on/off, case-insensitive,
  trimmed), `IsHostname`. Type semantics: int/float via strconv (float must
  be finite); port 1–65535; url needs a scheme and host or path (so
  `localhost:5432` is not a URL), schemes case-insensitive; email
  `x@y.tld`; hostname RFC 1123 labels or an IP; duration Go syntax; enum
  case-sensitive; json `json.Valid`; regex compiles. `pattern` is searched
  (anchor it yourself).
- **internal/generate** — (implemented) `Generate(p, Options{Example, App})
  *Result{Config, Example, counts}` for `envguard init`. Collects variables
  from templates, non-dynamic code usages, Compose refs and workflow refs
  (excluding `scanner.IsWellKnown(name, false)`). `InferType(name, samples)`:
  name first (PORT/*_PORT → port, *_URL/*_URI → url, *_EMAIL → email,
  *_HOST/*_HOSTNAME → hostname; dropped if a non-placeholder template sample
  contradicts it), else template samples (true/false/yes/no/on/off → bool,
  digits → int (1/0 too), decimals → float, `scheme://` → url), else string.
  `required: true` if any code read lacks a default; `sensitive: true` for
  `mask.IsSensitiveName`; a `# TODO: describe …` comment plus `# read in /
  declared in / referenced in` locations per variable. The example file lists
  variables read by code, grouped by the prefix before the first `_`
  (`GENERAL` otherwise), all with empty values (`# optional: the code has a
  default` when every read has one). **It never copies any value**: template
  values are only used for type inference; real env files are not read.
  Guarded by `TestInitNeverCopiesValues`.
- **internal/project** — (implemented) `Build(settings, inventory, Options)`
  reads and parses every env file/template (the only I/O; unreadable files get
  `ReadErr`, never content) into `Project{Apps, EnvFiles, Settings, Inventory}`.
  `App{Root, Groups}`; `EnvGroup{Environment, Files, Templates}`, groups sorted
  `default` first then by name. Helpers: `ApplicableTemplates(env, local)`
  (default templates + that env's; local templates only for local files),
  `AncestorApps`, `File(path)`, `EnvSelected(env)` (`--env`, aliases
  normalized), `IsIgnored(rule, file, line)` (inline directives). `Build` also
  runs the scanner: `CodeUsages` (`CodeUsage = scanner.Usage`),
  `ScannedFiles`, `UnscannableFiles`; helpers `UsagesUnder(root)` (app root
  and everything below; "." = all) and `DynamicUsages(root)`. It also parses
  Compose files (`ComposeFiles`; `ComposeRefs` = environment definitions with
  `Defines` + interpolations; env_file `Exists` via os.Stat) and workflows
  (`WorkflowFiles`; `WorkflowRefs` with `Defines` for env: keys), builds the
  **CI inventory** `CI{Secrets, Vars []CIItem{Name, Refs}}` (sorted, no
  GITHUB_TOKEN; for doctor), and runs `gitutil.Inspect` over all env files
  (`Git *GitStatus`, never nil; check `Available`). `Schema *Schema{File,
  Variables map[string]*SchemaVar{Name, Config, Spec, Line, Column}}` (nil
  without variables; `File` is relative to the root, symlinks resolved;
  `Location(name)`, `Names()`), `SchemaVar(name)`, `Sensitive(name)` (schema
  `sensitive` or sensitive-looking name) and **`DisplayValue(name, value)`:
  mask.Display plus schema `sensitive: true` — rules must use it for values**.
  `Options.ProcessEnv{Vars, Env, App}` switches to --process-env mode: real
  env files are dropped and replaced by one synthetic real file
  `ProcessEnvPath` = "(process environment)" (entries sorted, positions 0) in
  that app/environment group; the env filter is set to that environment;
  scanner, Compose, workflows and git are skipped (`Git.Reason` says so);
  `Project.ProcessEnv` is true. `Options.EnvFilesOnly` (used by `diff`) also
  skips the scanner, Compose, workflows and git: only env files are read.
- **internal/rules** — (implemented: engine + all rules EG001–EG016) `Rule` interface (ID, Name, DefaultSeverity,
  `Check(*project.Project) []findings.Finding`), registry (`Register` from
  `init()`, `All()` sorted, `Lookup`) — the only allowed global mutable state.
  `Run(p, rules)` runs rules concurrently, recovers panics into
  `*InternalError` naming the rule (panic values only shown for
  `runtime.Error`, so values can't leak) → exit 2, rejects findings without
  Message/Why/Fix (also an `*InternalError`), fills RuleID/RuleName/DocsURL
  and default Severity, skips rules set to `off`, then applies suppression.
  Rules are pure functions of `Project`: no I/O, no globals, no time, no
  randomness. One file per rule: `eg0NN_<name>.go`.
- **internal/findings** — (implemented) `Finding`, `Location`, `Severity`
  (int, `info < warning < error`, `ParseSeverity`, text (un)marshaling),
  `Sort`/`Compare` (file, line, col, rule, variable, message), `Dedupe`,
  `Count`, `AnyAtLeast`. `Suppression{Overrides, Ignores, Inline}.Apply`:
  severity overrides, then config ignores (rule / path glob / variable with
  `*`) and inline directives; returns the `Suppressed` count. An inline
  directive matches a finding's primary location **or any Related location**
  (so `# envguard-ignore EG001` above a template line silences "missing"
  findings that point to it).
- **internal/mask** — `Printable(s)` (terminal-safe text: control, format and
  line-separator characters and invalid UTF-8 become visible escapes; `\n`
  and `\t` kept; applied to everything that comes from file names or YAML
  keys) and `Clip(s, n)`, besides the value logic below. `IsSensitiveName(name)` (substrings
  like SECRET, TOKEN, PASSWORD, PRIVATE, CREDENTIAL, API_KEY, WEBHOOK, ...;
  whole tokens like KEY, PWD, AUTH, DSN, PASS) and `Display(name, value)`,
  **the only function allowed to turn a value into output text**. It shows a
  value only if the name is not sensitive and the value has ≤ 24 chars, one
  line, no control/bidi characters, no userinfo, no known credential prefix
  (`sk_live_`, `ghp_`, `AKIA`, ...) and doesn't look random (16+ chars mixing
  upper case, lower case and digits). URLs show `scheme://host/path` only
  (never userinfo, query or fragment; token-like path segments become `…`).
  Otherwise it returns `•••••• (N chars)`; an empty value is `(empty)`.
  **Any value `LooksLikeSecret` flags is always masked, whatever its name**
  (so a URL with a real password is fully masked, not shown as scheme://host).
  - `LooksLikeSecret(key, value) (kind, ok)` (`secret.go`): known formats
    anywhere in the value — PEM `BEGIN … PRIVATE KEY`, AWS access key IDs
    (AKIA/ASIA/ABIA/ACCA), GitHub `ghp_`/`github_pat_` (PAT) and
    `gho_`/`ghu_`/`ghs_`/`ghr_`, Slack `xox?-` tokens and webhook URLs, Stripe
    `sk_live_`/`rk_live_` (test keys are not secrets), Anthropic `sk-ant-…`,
    OpenAI-style `sk-…`, Google `AIza…`, SendGrid `SG.…`, Twilio `SK`+32 hex,
    JWTs, URLs with a real password — plus, **only for sensitive names**,
    values ≥ 20 chars without spaces and Shannon entropy ≥ 3.5. `kind` is a
    fixed description ("GitHub personal access token"), never value text.
  - `IsPlaceholder(key, value)`: empty, changeme/change-me, replace-me, todo,
    tbd, test, dummy, example, placeholder, secret, password, true/false, …;
    contains example/dummy/placeholder/changeme/replace-me/your-/your_; token
    test/todo/fake/sample/xxx; `<…>`, `[…]`, `{{…}}`, `${…}`; runs like
    `xxxx`, `***`, `00000000`, `...`; value equal to the key name; localhost
    URLs without credentials. URL passwords are not secrets when the host is
    local, the password equals the user name, or it is a placeholder.
    Provider-shaped placeholders (`sk_live_xxxx…`, `…EXAMPLE`) never count.
  - `Entropy(s)` is Shannon entropy in bits per character.
- **internal/report** — (implemented) Renders results. Pure functions of
  their input and `Options{Color, ASCII, Width, ShowDuration}`; no I/O except
  the writer, no env access (cmd decides color/Unicode/width).
  - `Build(p, findings, Meta) *Report`: tool/version, root (as given),
    `PathPrefix` (relative --root + "/", prepended to paths in github/sarif so
    they resolve from the checkout; empty for "." and absolute roots), stats
    (`Stats`: env files, templates, compose, workflows, source files per
    language, distinct variables, app roots, dynamic lookups), findings,
    suppressed count, matrix, CI inventory, scanner notes, rule metadata
    (`RuleInfos()` = registry + docs).
  - `Write(w, format, rep, opts)` dispatches: `Pretty` (findings grouped by
    file, `  12:1  ✗ error    EG001  message`, line:col right-aligned per
    file, summary `✗ 2 errors · ⚠ 3 warnings · 1 info · 14 files[ · N
    suppressed][ · 0.08s]`, then a hint to run doctor/explain), `Doctor`
    (header; Scanned; findings by severity, each with message, severity · rule
    · location, Why, numbered Fix steps (`fixSteps`: explicit `1.` lines, else
    one step per sentence, indented code attaches to the previous step),
    Evidence, Related; environment matrix; CI inventory; "Good to know"
    (dynamic lookups, suppressed, git skipped, ≤5 scanner notes); Next steps;
    without findings a short "All good" with stats), `JSON`
    (docs/json-output.md, `SchemaVersion` = 1), `SARIF` (2.1.0, all rules
    with short/full description from the docs, helpUri, help markdown,
    default level, tags; results with ruleIndex, `%SRCROOT%` relative URIs,
    region, relatedLocations, `partialFingerprints["envguard/v1"]` = sha256 of
    rule+file+variable (or message)+occurrence, independent of line numbers),
    `GitHub` (`::error|warning|notice file=…,line=…,col=…,title=EG001
    name::message%0A%0AWhy…%0A%0AFix…`, properties and data escaped per the
    workflow-command rules) and `GitHubSummary` (Markdown table, cells
    escaped, HTML neutralized, ≤100 rows).
  - Matrix: `BuildMatrix(p, app)` → `[]AppMatrix` (columns = environments
    with a real file honoring --env; template-only envs listed separately;
    cells `set|empty|missing|absent`; `missing` only for template variables
    that are required per `rules.IsRequired`; never values). `BuildDiff(p, app, a, b)`
    (aliases; a side without a real file uses its applicable templates;
    `ErrUnknownEnvironment` never echoes the argument), `MatrixText`,
    `DiffText`, `MatrixJSON`, `DiffJSON`.
  - `Markdown` renders rule docs for the terminal (headings, wrapped
    paragraphs, lists, indented code; styled with color, markers kept
    without), `RuleList` for `explain --list`.
  - **Text renderers work on `Report.printable()`**, a copy in which every
    name from the repository (paths, variable and environment names, messages)
    went through `mask.Printable`: terminal escape sequences, bidi overrides
    and invalid UTF-8 become visible escapes (`\x1b`, `\u202e`). JSON and
    SARIF keep raw strings (their encoders escape). Aligned name columns are
    clipped (`mask.Clip`, 48 characters; environments 24): one 500 kB variable
    name once padded a matrix to tens of gigabytes.
  - Styling (`style.go`): lipgloss with the fixed 16-color ANSI profile
    (follows the terminal theme), only when `Color`; Unicode symbols ✗ ⚠ • ✓
    · or ASCII `x ! i ok |`; with `ASCII` the whole output is transliterated
    (`Options.finish`). Prose wraps at `Width` (default 80, clamped 50–100);
    `` `code` `` spans are never broken.
- **docs** — Go package `docs` that embeds `docs/rules/*.md` with `go:embed`.
  Single source of truth for `envguard explain`, SARIF rule metadata and the
  website. Each rule has exactly one markdown file named
  `EGxxx-<rule-name>.md` with this fixed structure (parsed by `docs.Rule` /
  `RuleDoc.Section`; enforced by `TestEveryRuleHasDocs` in internal/report):
  `# EG001 missing-variable`, `**Default severity:** error`, a one-sentence
  summary line (used by `explain --list` and SARIF), then `## What it
  detects`, `## Why it matters`, `## Bad example`, `## Good example`, `## How
  to fix`, `## How to suppress`. Example findings are written in the pretty
  format. Never put real-looking secrets in docs.

### Data flow

```
flags + .envguard.yaml ──► config
                              │
--root ──► discovery ──► parser / scanner / compose / workflow / gitutil / schema
                              │
                              ▼
                        project.Project (immutable)
                              │
                              ▼
                     rules.Registry → []findings.Finding
                              │  sort, suppress, threshold
                              ▼
                     report (via mask) ──► stdout
                              │
                              ▼
                     cmd/envguard: error → exit code
```

---

## 4. Finding model

```go
type Finding struct {
    RuleID   string     // "EG001"
    RuleName string     // "missing-variable"
    Severity Severity   // error | warning | info
    Message  string     // one line, no trailing period required, no values
    Variable string     // env var name, if applicable
    File     string     // slash-separated, relative to --root
    Line     int        // 1-based; 0 if not applicable
    Column   int        // 1-based; 0 if not applicable
    Evidence string     // optional; MUST come from internal/mask
    Why      string     // 1-3 sentences: why this matters
    Fix      string     // concrete steps; may contain a code snippet
    Related  []Location // other relevant positions (e.g. where declared/used)
    DocsURL  string     // link to the rule documentation
}

type Location struct {
    File   string
    Line   int
    Column int
}
```

Severity ordering: `info < warning < error`.

---

## 5. Rule catalog

| ID | Name | Default severity | Triggers when |
|---|---|---|---|
| EG001 | missing-variable | error | Declared in a template or schema for an environment, but absent in that environment's real env file. |
| EG002 | undocumented-variable | warning | Present in a real env file but in no template or schema. |
| EG003 | used-but-undeclared | error (info if every read is optional, or the repo declares nothing) | Code reads a var that no env file, template, schema, compose file or workflow declares. |
| EG004 | declared-but-unused | info | Declared but never read by code, compose, or workflows. |
| EG005 | empty-required-value | warning | A required variable (per schema or template convention) has an empty value. |
| EG006 | duplicate-key | warning | The same key appears more than once in one env file. |
| EG007 | syntax-error | error | An env file (or compose/workflow/schema) cannot be parsed. |
| EG008 | env-file-committed | error | A real (non-template) env file is tracked by git. |
| EG009 | env-file-not-ignored | warning | A real env file is not covered by `.gitignore`. |
| EG010 | possible-secret-in-template | error | A template or tracked file contains a value that looks like a real secret. |
| EG011 | compose-undefined-variable | warning | Compose interpolates `${VAR}` with no default and no definition anywhere. |
| EG012 | ci-secret-undocumented | info | Workflows reference `secrets.X` / `vars.X` not documented anywhere. |
| EG013 | environment-drift | warning | Templates of different environments disagree on variable names. |
| EG014 | invalid-value | error | A value violates its schema type or constraints. |
| EG015 | unsafe-production-value | warning | e.g. `DEBUG=true` in production; only via schema or an opt-in profile. |
| EG016 | unresolved-reference | warning | `${VAR}` inside an env file references an undefined variable. |

Rule IDs are permanent: never renumber or reuse an ID. Each rule has a doc at
`docs/rules/EGxxx-<name>.md`, golden tests, and positive + negative fixtures.

**Rule test layout** (enforced by `TestRuleFixtures` / `TestRegistry` in
`internal/rules`): `testdata/fixtures/<ID>/positive/` must produce at least
one finding, `testdata/fixtures/<ID>/negative/` must produce none (each is
usually its own app root via a `package.json`), and the output of that rule
alone is snapshotted in `testdata/fixtures/<ID>/golden.txt` (every field,
including Why/Fix wording). A registered rule without a fixture or a doc
fails the tests. `cmd/envguard/testdata/check.golden` snapshots `check` over
the whole fixture tree. `TestNoValuesInMessages` verifies that no fixture
value appears in any Message/Why/Fix.

**Template conventions used by rules:**
- *Required*: a template variable is required if the template gives it a
  non-empty value, **or** its line is not marked `# optional` (inline comment
  `optional`, `optional: ...`, `optional - ...`, case-insensitive). So only
  `KEY= # optional` is optional. EG001 and EG005 skip optional variables.
- *Applicable templates* for a real file: the app's default templates plus its
  environment's templates (`.env.production` ← `.env.example` +
  `.env.production.example`).
- *Schema requirements* (EG001, EG005): a schema variable with `required:
  true` is required in the environments it lists (all if none), but only in
  app roots that reference it (their templates or code), in the root app, and
  in --process-env mode — so a monorepo-wide schema does not demand every
  variable in every app. Template-based requirements are reported first; a
  schema finding is not duplicated. Schema variables count as documented for
  EG002 and declared for EG003.
- EG014 validates the last definition of each key in real env files (and the
  process environment), never templates; empty values skipped. Message:
  `X in F has the invalid value <DisplayValue>: <reason>`, or without the value
  (`has an invalid value: <reason>`) when `Project.Sensitive(X)`.
- EG015 runs only on the `production` environment and only via schema
  `production:` rules or `profiles: [production-safety]` (never by default).
  A schema rule for a variable replaces the profile check for it. Profile
  built-ins: DEBUG/DJANGO_DEBUG/APP_DEBUG/FLASK_DEBUG truthy (ParseBool),
  NODE_ENV/RAILS_ENV ≠ production, APP_ENV local/development/dev, *_URL and
  *_HOST pointing to localhost/127.0.0.1/0.0.0.0/::1 (in --process-env mode
  only for variables declared in a template or the schema, so the runner's
  own variables don't count). Localhost URLs are reported without the value.
- In messages, `where(f)` renders the process environment as "the process
  environment".
- EG001 skips local files (`.env.local`, `.env.<env>.local`) and never reports
  a missing env file. EG002 counts a variable as documented if any template in
  the app root or an enclosing app root declares it; with no template at all
  it reports the file once. EG005 checks the last definition of a key.
  EG006 reports each repeated definition (Related = the others; Evidence =
  masked values if they differ). EG007 keeps parser warnings (unquoted
  whitespace, bad directives) at warning. EG013 compares only non-default,
  non-local templates and needs two or more environments; default-template
  variables are shared. EG016 resolves against the same file, all files and
  templates of the default environment and of the file's own environment,
  well-known OS vars (HOME, PATH, ...) and `scanner.ignore_variables`;
  `${VAR:-x}` never triggers.
- EG003 counts a variable as declared if **any** env file or template in the
  repository, a config/schema variable, a Compose definition or a workflow
  `env:` entry declares it (repo-wide for precision in monorepos). One finding
  per (app root, variable) at the first usage, up to 3 more usages in
  Related. **Severity: `error` when the code needs the value; `info` when
  every usage is optional (`Usage.HasDefault`: a default, a comparison, an
  API that cannot fail such as Laravel `env()`), and also `info` when the
  repository declares nothing anywhere** (a CLI or library with no env file,
  template, Compose/workflow definition or schema variable). Found by
  running real projects: stock Laravel config alone has dozens of optional
  `env()` reads. Never: dynamic or inferred usages (`Usage.Certain()`),
  `scanner.IsWellKnown`, `scanner.ignore_variables`. The Fix names the app's
  default template (or the nearest enclosing app's, or a new
  `<app>/.env.example`) and, for optional reads, suggests `X= # optional`.
- EG004 reports template variables nothing reads (code anywhere, `${VAR}`
  references in env files, Compose/workflow refs, inferred Go envPrefix
  names). It skips app roots with zero scanner usages in or below them
  (`rules.SkippedForUnused`, logged by `check --verbose`), **the whole
  repository when any code reads the whole environment**
  (`rules.BulkEnvironmentRead`: `schema.parse(process.env)`, so the names are
  in a schema), well-known names and ignore_variables, and mentions dynamic
  lookups in Why. Known blind spots (documented in docs/how-it-works.md): Rails
  ERB/YAML config, reads by frameworks themselves. Schema-only variables are not checked yet (no
  positions until internal/schema exists). Compose `environment:` entries and
  interpolations and all workflow refs count as "used".
- EG007 also reports Compose and workflow files that are not valid YAML.
- EG008 (tracked real env file) and EG009 (untracked, not ignored real env
  file) need `Git.Available`; EG008's Fix always contains `git rm --cached
  <file>`, the .gitignore entry and the warning that values stay in history
  and must be rotated. Templates are never reported by either.
- EG010 checks every entry of templates (always) and of git-tracked real env
  files with `mask.LooksLikeSecret`; the message names the kind with the right
  article ("an AWS access key ID", "a URL with…"), Evidence is
  `detected: <kind>`, never the value.
- EG011 per Compose file: interpolations without default (Required ones
  included) not defined in `Dir/.env` (if it exists), templates in that
  directory or in the file's app root and ancestors, or config variables;
  well-known host vars and ignore_variables skipped; one finding per (file,
  variable), message says whether the adjacent .env is missing. Plus missing
  `env_file` entries unless `required: false` (distinct message).
- EG012 (info): `Project.CI` secrets and vars not declared in any template
  in the repository or config variables; one per name at its first reference.
- Fix texts name exact files and lines, e.g. "Add `DATABASE_URL=` to
  .env.production (it is declared in .env.example:4)".

### Environment model

- Real env files: `.env` (environment `default`), `.env.<env>` (environment
  `<env>`), `.env.<env>.local` (environment `<env>`, local override).
- Templates: any env file whose last suffix is `.example`, `.sample`,
  `.template`, or `.dist` (e.g. `.env.example` → `default`,
  `.env.production.example` → `production`).
- Schema can declare environments explicitly and override the above.

---

## 6. Exit codes

| Code | Meaning |
|---|---|
| 0 | No findings at or above `--fail-on` (default: `error`), or `--fail-on never`. |
| 1 | At least one finding at or above the threshold. |
| 2 | Runtime or configuration error (bad flags or arguments, an unknown `--env`/`--app`/`--format`, invalid `.envguard.yaml`, unreadable root, an internal error caught by the panic guard). |

The mapping from errors to exit codes lives **only** in `cmd/envguard`.
Other packages return errors (sentinel or typed, wrapped with `%w`); they never
call `os.Exit`.

---

## 7. Conventions

- **Go**: latest stable. `go.mod` declares the minimum: currently **1.26**,
  forced by go-git (the gitignore matcher).
- **Dependencies** (each justified): cobra (CLI), charmbracelet/lipgloss v1
  (+ muesli/termenv, charmbracelet/x/term: styling, Windows VT mode, TTY
  detection; none import `net`), go.yaml.in/yaml/v3 (the
  maintained continuation of gopkg.in/yaml.v3 with the same API and node
  positions; config, compose, workflow — use it, not gopkg.in/yaml.v3),
  bmatcuk/doublestar/v4 (globs), go-git/go-git/v5 — only
  `plumbing/format/gitignore` is compiled in. The `git` binary is optional and
  used only through internal/gitutil. Test-only: rogpeppe/go-internal
  (testscript), santhosh-tekuri/jsonschema/v6 (SARIF schema validation).
- **CLI**: `github.com/spf13/cobra`. Keep dependencies minimal; every new
  dependency must be justified and must not import `net`/`net/http` into the
  production binary.
- **Errors**: wrap with `fmt.Errorf("context: %w", err)`; check with
  `errors.Is` / `errors.As`. Error strings lowercase, no trailing punctuation.
  Errors never contain raw values.
- **State**: no global mutable state except the rule registry.
- **Tests**: table-driven. Golden files live in each package's own
  `testdata/` (e.g. `internal/parser/testdata/parser/*.env` →
  `*.golden.json`) and are compared with `internal/testutil.Golden`, which
  defines the `-update` flag. Run updates per package, e.g.
  `go test ./internal/parser -run TestGolden -update` (`./...` would fail in
  packages that don't import testutil). Review golden diffs before
  committing. `internal/testutil` may only be imported from `_test.go` files.
  Fixture trees that need a nested `.gitignore` store it as `_gitignore`;
  tests copy the tree to `t.TempDir()` and rename it (a real nested
  .gitignore would make git drop the fixture files it ignores). Fixture file
  names must be portable: no case-only differences, no trailing dots.
  CLI end-to-end tests are testscript scripts in
  `cmd/envguard/testdata/script/*.txtar` (`TestScripts`; one script per
  command; the custom `exitcode N envguard …` command asserts exact exit
  codes; `$WORK/demo` is a git-free copy of examples/demo-app). `TestMain`
  unsets GITHUB_ACTIONS/GITHUB_STEP_SUMMARY/NO_COLOR for in-process tests.
  Report goldens: `internal/report/testdata/golden/<project>/<format>.golden`
  for the demo app, several rule fixtures and a clean project (every format,
  ASCII, color, matrix, diff), regenerate with `go test ./internal/report
  -update`. SARIF output is validated against the official schema vendored
  at `internal/report/testdata/sarif-schema-2.1.0.json` (no network).
  Symlinks, FIFOs and >1 MiB files are created at runtime, never committed;
  symlink tests skip on Windows when unsupported, FIFO tests are `unix`-only.
  Use `t.TempDir()` for filesystem tests. **Fuzz targets** (all run by `make
  fuzz`): `internal/parser` (dotenv), `internal/scanner` (every language's
  detectors), `internal/compose`, `internal/workflow`, `internal/config`
  (the strict loader). Fuzz harnesses must themselves be linear-time
  (precompute, don't rescan). Crashers found by fuzzing are kept as regression
  seeds in `testdata/fuzz/` (so far: lone-CR line counting in the Compose
  harness, a mapping used as a YAML key under `env:`, and a duplicate config
  key giving a multi-line error). Before v0.1.0 every target ran 5 minutes
  with no crash (≈ 8–15 million executions each).
  - **Real-world fixtures** (`testdata/realworld/<name>/`: Next.js, Django,
    a Go service with caarlos0/env, Laravel, Rails, a pnpm monorepo, a CLI with
    no env files; an empty directory is created at run time) are written like
    real projects, with `PLANTED.txt` listing the 1–3 real problems each
    contains. `TestRealWorldGolden` compares `check` output (each fixture
    copied into a git repository where `.gitignore` applies) with reviewed
    goldens (`cmd/envguard/testdata/realworld/*.golden`; every finding in them
    was classified by hand as a true positive or a documented limitation);
    `TestRealWorldPlantedProblems` requires every planted problem to be
    reported. **Any false positive found on a real project is a bug: fix the
    cause, add a regression test** (see `internal/rules/review_test.go` and
    the scanner tables), then update the golden.
  - **Security tests** (`cmd/envguard/security_test.go`, `readonly_test.go`,
    `hidden_chars_test.go`): imports (no network or telemetry packages, only
    gitutil runs programs), the panic guard, `init` dry-run/force, `env_file`
    paths outside the root, the hostile repository (huge, binary, nested,
    invalid UTF-8, symlinks, escape-sequence file names; no crash, bounded time,
    no control characters in text output), no hidden/bidi characters in
    sources (write them as `\u202e`), CRLF/BOM equivalence
    (`TestWindowsLineEndingsAndBOM`). `make security` runs gosec (every
    remaining finding has a `#nosec Gxxx -- reason` comment; test helpers
    excluded) and govulncheck.
  - **Consistency tests** (`consistency_test.go`): the rule catalog in this file
    matches the registry and `explain --list`, documents mention only real rule
    IDs (CONTRIBUTING may use the next free ID), every real JSON key is in
    docs/json-output.md, tests named in README/SECURITY/CONTRIBUTING/CLAUDE
    exist (`TestDocsNameRealTests`).
- **Fixtures and secrets**: fixtures never contain real-looking secrets as
  literals. Build them at runtime by string concatenation
  (e.g. `"AKIA" + "IOSFODNN7" + "EXAMPLE"`) so GitHub push protection and
  secret scanners never trigger on this repo. Use `testutil.FakeSecret(kind,
  seed, marker)` (kinds: aws, github, slack, stripe, openai, anthropic, google,
  sendgrid, twilio, pem, jwt, url, entropy) and `testutil.Canary(i)` (unique,
  contains `CNRY%04d`). In static fixture files write `{{secret:KIND}}`:
  `testutil.CopyFixture` expands it at copy time (unexpanded, it is a
  placeholder EG010 ignores).
  For a value that merely has a provider's *shape* but must NOT be flagged
  (a negative fixture, e.g. `sk_live_` followed by `x` characters) write
  `{{placeholder:KIND}}` (`testutil.Placeholder`, currently `stripe`). GitHub
  push protection blocks a push containing even an obvious placeholder in the
  shape of a Stripe key; `TestNoSecretShapedLiteralsInRepository` fails on any
  complete credential-shaped literal in a committed file.
- **Rule fixtures run on copies**: `testutil.CopyFixture` copies the tree to
  `t.TempDir()` (renaming `_gitignore`, expanding secret markers) and, if the
  fixture root has a `git-tracked.txt` manifest (one path per line), runs `git
  init` and `git add -f` on those paths (skipping the test without git). Tests
  must never depend on the git state of this repository (once committed, its
  fixture `.env` files would be "tracked" and `!**/testdata/**` un-ignores
  them).
- **Paths**: use `path/filepath` internally; output paths are always
  slash-separated (`filepath.ToSlash`) and relative to `--root`.
- **Portability**: must work on Linux, macOS and Windows, including CRLF line
  endings and UTF-8 BOMs. CI runs on all three.
- **Version info**: `main.version`, `main.commit`, `main.date` set via
  `-ldflags "-X main.version=... -X main.commit=... -X main.date=..."`
  (GoReleaser, `make build`). Values ldflags left at their defaults are filled
  from `debug.ReadBuildInfo` (`cmd/envguard/version.go`): module version
  (`go install …@v1.2.3` → `1.2.3`, local builds get a pseudo-version),
  `vcs.revision` (12 chars, `-dirty` if modified), `vcs.time`.
- **Output**: stdout for results, stderr for logs/errors. `--no-color` and the
  `NO_COLOR` env var disable color; color is also off when stdout is not a TTY.

### Make targets

| Target | What it does |
|---|---|
| `make build` | Build `bin/envguard` with version ldflags. |
| `make test` | `go test -race ./...` |
| `make lint` | `golangci-lint run` (installs the pinned version into `bin/` if missing). |
| `make security` | gosec + govulncheck (installed into `bin/`; needs the network once). Also a CI job. |
| `make fuzz` | Run every `Fuzz*` target for `FUZZTIME` (default `30s`) each, with `FUZZMINIMIZETIME` (default `1s`) so minimization does not stall the run. |
| `make cover` | Coverage profile + HTML report. |
| `make snapshot` | GoReleaser (pinned, installed into `bin/`) `release --snapshot --clean`: archives, checksums, SBOMs (if `syft`), deb/rpm/apk, Homebrew cask + Scoop manifest files, Docker images (if a daemon runs; `sign` is always skipped locally), then the npm packages in `dist/npm` with a smoke test. Publishes nothing. |
| `make release-check` | `goreleaser check` (deprecated keys fail). |
| `make npm` | Rebuild + smoke-test the npm packages from an existing `dist/`. |
| `make shellcheck` | `shellcheck install.sh`. |
| `make clean` | Remove build outputs. |

### Distribution

All of it lives outside the binary and must never add network code to it.

- `.goreleaser.yaml` (GoReleaser v2.19, pinned in Makefile and workflows):
  6 targets (linux/darwin/windows × amd64/arm64), `-trimpath`, ldflags,
  `mod_timestamp` from the commit. Archive names
  `envguard_<version>_<os>_<arch>.tar.gz|.zip` are a contract used by
  install.sh, action.yml and scripts/npm/build.mjs. `checksums.txt` signed
  keylessly (`checksums.txt.sigstore.json`), SPDX SBOM per archive, nfpm
  deb/rpm/apk, `dockers_v2` multi-arch image (`Dockerfile`: distroless
  static nonroot, `WORKDIR /repo`, entrypoint envguard, cmd check; image
  owner from `GITHUB_REPOSITORY_OWNER`, lower-cased) signed with cosign,
  `homebrew_casks` (GoReleaser deprecated `brews`) → homebrew-tap/Casks,
  `scoops` → scoop-bucket.
- `.github/workflows/release.yml` (v* tags, `permissions: {}` + per job):
  goreleaser job (contents/packages write, id-token for cosign; also moves
  the major tag `v1` for stable releases), npm job (id-token for
  provenance, `NPM_TOKEN`, `environment: npm`, skips already-published
  versions, prereleases get dist-tag `next`).
- npm (esbuild pattern): `npm/envguard/bin/envguard.js` launcher template
  (no postinstall, no network; `ENVGUARD_BINARY` override; clear errors for
  unsupported platforms or a missing optional dep) and
  `scripts/npm/build.mjs` (reads `dist/artifacts.json`, writes
  `dist/npm/<name>` + `<prefix>-<os>-<cpu>` with `os`/`cpu`, and
  `publish-order.txt`; `--smoke` runs the launcher). Default name
  `envguard-cli` (the `@envguard` scope is taken; `envguard` collides with
  `env-guard`); override with `--name`/`--prefix` or `NPM_PACKAGE_NAME` /
  `NPM_PLATFORM_PREFIX`.
- `install.sh`: POSIX sh, shellcheck-clean (CI), curl or wget, verifies
  SHA-256 against checksums.txt (optional cosign with
  `ENVGUARD_VERIFY_SIGNATURE=1`), `ENVGUARD_INSTALL_DIR`,
  `ENVGUARD_RELEASES_URL` (mirrors; also used to test against a snapshot).
- `action.yml` (composite): resolves the version, caches the binary in the
  tool cache (`actions/cache`), installs via install.sh (checksum), runs
  `envguard --root <working-directory> <args>` (args split without eval;
  `fail-on` appended for check/doctor unless args has `--fail-on`),
  optional SARIF + `github/codeql-action/upload-sarif`.
  `.github/workflows/action-test.yml` tests it on 3 OSes (skips until a
  release exists).
- `.pre-commit-hooks.yaml`: `envguard` hook, `language: golang`,
  `pass_filenames: false`, `always_run: true`.
- CI (`ci.yml`): test (3 OSes), lint, shellcheck, **dogfood** (`go run
  ./cmd/envguard check` on this repo with the root `.envguard.yaml`, which
  excludes fixtures/examples and documents EnvGuard's own env vars and CI
  secrets — add new ones there), release dry run (goreleaser check +
  snapshot without signing, npm build + smoke test, install.sh against the
  snapshot served locally).
- docs/installation.md documents every method and verification.

### Documentation and community files

- **Docs cannot drift.** `cmd/envguard/docs_test.go` parses README.md,
  CONTRIBUTING.md and docs/**/*.md. An HTML comment on the line before a code
  fence marks it: `<!-- envguard-example -->` (block = `$ envguard ARGS` + exact
  output; `excerpt` = the lines appear in order, `...` skips, `exit=N` checks
  the exit code; run against a git-committed copy of examples/demo-app with
  `--root`), `<!-- envguard-file path=src/a.js -->` (lines exist, in order, in
  that demo-app file), `<!-- envguard-config -->` (a complete, valid
  `.envguard.yaml`). `TestDocLinks` checks relative links. The README rules
  table between `rules-table:start/end` is generated:
  `go test ./cmd/envguard -run TestReadmeRulesTable -update`. Changing output
  wording or the demo app means updating the docs that quote it. `TestDemoTape`
  keeps `docs/demo.tape` (VHS; `make demo` renders `docs/demo.gif`) to the four
  documented commands in under 20 s.
- README.md is the landing page (order: logo, tagline, badges, demo GIF, "Try it
  in 10 seconds", "What it catches", CI, config, rules, "Safe by design",
  comparison, FAQ). Its "Safe by design" table names the test behind every
  promise: keep that true when tests are renamed. The npm package is
  `envguard-cli` (say so wherever `npx` appears).
- `TestCommandsDoNotModifyTheRepository` and `TestOnlyDocumentedFilesWriteToDisk`
  (`cmd/envguard/readonly_test.go`) enforce principle 2: only `init.go` and
  `output.go` may call file-writing APIs.
- Reference docs: docs/configuration.md, docs/ci.md, docs/how-it-works.md
  (Mermaid pipeline, scanner patterns, known limitations), docs/json-output.md,
  docs/installation.md. Community: CONTRIBUTING.md (step-by-step "add a rule" /
  "add a language detector"), SECURITY.md, CODE_OF_CONDUCT.md (Contributor
  Covenant 2.1), CHANGELOG.md (Keep a Changelog), `.github/ISSUE_TEMPLATE/*`,
  PR template, dependabot (gomod, github-actions, docker; npm is commented out
  because the npm manifests are generated into dist/npm), docs/roadmap.md and
  docs/roadmap/good-first-issues.md (drafts to open as issues).
- `examples/demo-app` has deliberate problems for most rules (EG008 appears only
  when its env files are committed). All its credentials are obvious fakes
  (`demo-only-pa55word`); never use a value that matches a real provider format.

---

## 8. Definition of done (every task)

- [ ] `make test`, `make lint` and `make security` pass.
- [ ] New behavior has tests (table-driven; goldens updated deliberately).
- [ ] `CLAUDE.md` updated if architecture, rules, exit codes or conventions changed.
- [ ] No secret value can reach any output (terminal, JSON, SARIF, GitHub
      annotations, errors, verbose logs) — all value text flows through `internal/mask`.
- [ ] No new network-capable imports in non-test code.
- [ ] Output remains deterministic.

---

## Status

- Implemented: everything in the architecture above — all packages, all
  rules EG001–EG016, all commands (`check`, `doctor`, `diff`, `explain`,
  `init`, `version`; no command = doctor) and all report formats (pretty,
  doctor, json, sarif, github + job summary).
- `check` flags: `--env` (comma-separated or repeated, aliases allowed),
  `--app`, `--fail-on error|warning|info|never` (also `fail_on` in config;
  flag wins), `--quiet` (no stdout; exit code only), `--format`, `--output`,
  `--process-env`. `--verbose` logs config, phase timings, skipped paths with
  reasons, unreadable sources, the dynamic lookup count, app roots where EG004
  is skipped, why git checks are skipped, and the rules run, to stderr. A
  truncated walk prints a warning.
  - `--process-env` (cmd reads `os.Environ()`; values only reach output via
    mask): checks the process environment against the templates and schema
    of one `--env` (more than one is an error; none = default) of one `--app`
    (default `.`). Only EG001 (unset), EG005 (empty), EG014 and EG015 run;
    file-based rules are skipped. Dockerfile/entrypoint usage:
    `envguard check --process-env --env production --quiet && exec "$@"`
    (README).
  - `--app <dir>`: must be a discovered app root (error lists known roots,
    never echoes the argument). In file mode it filters findings to that app.
- `envguard init` (`cmd/envguard/init.go` + internal/generate): ignores
  any existing config (uses defaults), discovers and builds the project, then
  writes `.envguard.yaml` (refuses if `.envguard.yaml` or `.envguard.yml`
  exists unless `--force`) and with `--example` `<app>/.env.example` (kept if
  it exists unless `--force`). `--dry-run` prints both to stdout. `--app`
  limits to one app root. Prints a summary and next steps. The generated
  config always passes strict validation (tested).
- `examples/demo-app`: a Node API + Python worker with Dockerfiles, Compose
  and a deploy workflow, broken on purpose; triggers every rule except
  EG008/EG009 (which need git state; committed in a clone, EG008 fires).
  `TestDemoCoversMostRules` guards that. README examples quote its output.
- Not implemented yet: EG004 for schema-only variables. Known limitations are
  listed in docs/how-it-works.md ("Known limitations"): keep that list and the
  README FAQ in step with reality.
- **v0.1.0 release audit** (what was checked and fixed before tagging): real
  projects (`testdata/realworld`) exposed false positives that are now fixed
  (optional reads were errors, Laravel `env()`, django-environ in split
  settings, Go `envPrefix`, zod `parse(process.env)`, ambient variables such
  as `XDG_*`/`VERCEL`/`VITEST`, comparisons); fuzzing found three bugs;
  a hostile repository exposed terminal-escape injection through file names
  and a quadratic table width (both fixed, tests added); an `--env` typo
  silently passed (now an error). Version comes from the git tag only
  (GoReleaser ldflags, or the Go build info for `go install`).

# EG003 used-but-undeclared

**Default severity:** error (info when the code has a default, or the repository declares nothing)

The code reads an environment variable that no env file, template, schema, Compose file or workflow declares.

## What it detects

The scanner finds reads in JavaScript/TypeScript (`process.env.X`,
destructuring, `import.meta.env`, Deno, Bun), Python, Go (including
`env:"X"` struct tags), Ruby, PHP, Rust, Java/Kotlin, C#, Elixir and shell
scripts. Matches inside comments and strings are ignored.

- A variable is declared if any env file or template anywhere in the
  repository, a schema variable, a Compose definition or a workflow `env:`
  entry declares it (repository-wide, so a monorepo can share one template).
- One finding per app root and variable, at the first usage, with up to
  three more usages as related locations.
- **Severity.** `error` when the code needs the value. `info` when every
  read is optional: it supplies a default (`|| "x"`, `os.getenv("X", "d")`),
  compares the value (`process.env.ANALYZE === "true"`, `os.Getenv("DEBUG") == "1"`),
  or uses an API that cannot fail (Laravel's `env()` returns null). Also
  `info` when the repository declares no variable anywhere (no env file,
  template, Compose or workflow `env:` entry, or schema): a CLI or library
  that reads a few optional variables has nothing to disagree with, so you
  get a suggestion to document them, not a failing build.
- Never reported: dynamic lookups such as `process.env[name]`; names the
  scanner only inferred (Go `envPrefix` structs); variables the OS, the shell,
  a test runner or a platform provides (`PATH`, `HOME`, `XDG_*`, `LC_*`,
  `EDITOR`, `CI`, `NODE_ENV`, `VITEST`, `GITHUB_*`, `VERCEL`, `VERCEL_*`,
  `AWS_REGION`, ...; `PORT` only with a default; AWS credentials are
  deliberately not on this list); names in `scanner.ignore_variables`.

## Why it matters

Nothing tells a new teammate, a CI job or a fresh deploy target that the
variable must be set, so the code reads an empty value at runtime. When the
code has a default, nothing breaks, but the variable is invisible to anyone
configuring the app and the default may be wrong for production.

## Bad example

```js
// src/app.js
const db = connect(process.env.DATABASE_URL);
```

```text
src/app.js
  2:32  ✗ error    EG003  DATABASE_URL is read by the code but not declared in any env file, template or schema
```

## Good example

```dotenv
# .env.example
DATABASE_URL=
```

## How to fix

1. Add `NAME=` to the app's template (the finding names the exact file).
   If the code works without it, write `NAME= # optional`.
2. Set the variable in each real env file.
3. If the platform provides it, add it to `scanner.ignore_variables`:

```yaml
version: 1
scanner:
  ignore_variables: [RAILWAY_*, DYNO]
```

## How to suppress

This finding points at source code, so an inline `# envguard-ignore EG003`
or `# envguard-ignore-file EG003` in an env file has no effect on it. Use
`scanner.ignore_variables` (above) or `.envguard.yaml`:

```yaml
version: 1
rules:
  EG003: warning   # or info, or off (it is info when the read is optional)
ignore:
  - rule: EG003
    path: "scripts/**"
    variable: DEPLOY_TOKEN
    reason: provided by the deploy runner
```

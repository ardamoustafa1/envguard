# EG004 declared-but-unused

**Default severity:** info

A template declares a variable that no code, env file reference, Compose file or workflow reads.

## What it detects

A template variable is reported when nothing reads it:

- no code usage anywhere in the repository,
- no `${VAR}` reference in an env file,
- no Compose `environment:` entry or interpolation, and no workflow
  reference.

EG004 stays silent when it cannot know what is read:

- App roots where the scanner found no env var usage at all are skipped:
  their language is probably not supported, and every variable would look
  unused.
- When the code hands the whole environment to a validator
  (`schema.parse(process.env)`, `createEnv({ runtimeEnv: process.env })`,
  `function load(source = process.env)`), the names live in a schema, so the
  rule is skipped in the whole repository.
- When an app has dynamic lookups (`process.env[name]`), the finding says so,
  because those lookups may read the variable.

`envguard check --verbose` explains each skip. Variables read through a Go
`envPrefix` struct (caarlos0/env) count as read: every name in such a file is
combined with every prefix of the file. django-environ calls (`env("X")`,
`env.bool("X")`) are recognized in every module of an app whose dependency
files name `django-environ`. Well-known platform variables and
`scanner.ignore_variables` are never reported. Schema-only variables are not
checked yet.

**Known blind spots** (the variable is read, but EG004 cannot see it): reads
in Rails `config/*.yml` files through ERB (`<%= ENV["X"] %>`), reads by a
framework or library itself (`RAILS_MASTER_KEY`, Laravel's `BCRYPT_ROUNDS`),
and reads in languages the scanner does not know. List such variables in
`scanner.ignore_variables`.

## Why it matters

Unused variables make templates longer and send people looking for values
nothing needs. They are often left over after a refactor.

## Bad example

```dotenv
# .env.example
DATABASE_URL=
LEGACY_CACHE_HOST=
```

```text
.env.example
  2:1  • info     EG004  LEGACY_CACHE_HOST is declared in .env.example but never read by the code, env files, Compose files or workflows
```

## Good example

```dotenv
# .env.example
DATABASE_URL=
```

## How to fix

1. Remove the variable from the template and the real env files.
2. If a tool outside the scanned code reads it (Prisma's `schema.prisma`, a
   Dockerfile, a framework), add it to `scanner.ignore_variables`.

## How to suppress

Put an inline directive on the line above the declaration in the template,
or a file-level directive in the template:

```dotenv
# .env.example
# envguard-ignore EG004 -- read by prisma/schema.prisma
DIRECT_URL=
```

```dotenv
# envguard-ignore-file EG004
```

In `.envguard.yaml`:

```yaml
version: 1
rules:
  EG004: off   # or warning, or info
ignore:
  - rule: EG004
    path: "infra/**"
    variable: DIRECT_URL
    reason: read by Prisma, not by scanned code
```

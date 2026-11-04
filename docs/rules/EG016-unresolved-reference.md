# EG016 unresolved-reference

**Default severity:** warning

A `$VAR` or `${VAR}` reference inside an env file points to a variable that nothing defines.

## What it detects

References in unquoted and double-quoted values of real env files and
templates are checked. A reference resolves if any of the following is true:

- The variable is defined in the same file.
- It is defined in any env file or template of the app's default environment
  or of the file's own environment. Loaders such as Next.js, Vite and Docker
  Compose load `.env` together with `.env.<env>`.
- It is a well-known process variable (`HOME`, `PATH`, `USER`, ...).
- It is listed in `scanner.ignore_variables`.

References with a default (`${VAR:-x}`, `${VAR-x}`), `$$` and `\$` escapes,
and references inside single quotes are never reported. References are never
expanded.

## Why it matters

Loaders that expand references substitute an empty string, so the
referencing value silently ends up incomplete (for example a URL without a
host).

## Bad example

```dotenv
# .env
DB_USER=app
DATABASE_URL=postgres://${DB_USER}@${DB_HOST}/app
```

```text
.env
  3:36  ⚠ warning  EG016  DATABASE_URL references DB_HOST, which is not defined in .env or the env files loaded with it
```

## Good example

```dotenv
# .env
DB_USER=app
DB_HOST=localhost
DATABASE_URL=postgres://${DB_USER}@${DB_HOST}/app
```

## How to fix

1. Define the variable in the same file or an env file loaded with it.
2. Or give the reference a default: `${DB_HOST:-localhost}`.
3. If the process environment provides it, add it to
   `scanner.ignore_variables`.

## How to suppress

Put an inline directive on the line above the entry, or a file-level
directive in the file:

```dotenv
# .env
# envguard-ignore EG016 -- DB_HOST is injected by the platform
DATABASE_URL=postgres://app@${DB_HOST}/app
```

```dotenv
# envguard-ignore-file EG016
```

In `.envguard.yaml`:

```yaml
version: 1
rules:
  EG016: info   # or warning, or off
ignore:
  - rule: EG016
    path: "deploy/**"
    variable: DB_HOST
    reason: set by the container platform
```

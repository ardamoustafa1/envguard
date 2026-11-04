# EG002 undocumented-variable

**Default severity:** warning

A real env file sets a variable that no template or schema documents.

## What it detects

- A variable in a real env file (`.env`, `.env.production`, ...) counts as
  documented if any template of the same app root, or of an enclosing app
  root (for example a shared `.env.example` at the root of a monorepo),
  declares it, or if `.envguard.yaml` declares it under `variables:`.
- When the app has no template at all, the file is reported once
  ("defines N variables but no template documents them") instead of once per
  variable, and the fix contains a starter template.

## Why it matters

Teammates, CI and deploy targets learn which variables exist from the
templates. An undocumented variable is missing wherever the local file is not
copied.

## Bad example

```dotenv
# .env.example
DATABASE_URL=
```

```dotenv
# .env
DATABASE_URL=postgres://localhost/app
FEATURE_FLAG=on
```

```text
.env
  2:1  ⚠ warning  EG002  FEATURE_FLAG is set in .env but not documented in any template
```

## Good example

```dotenv
# .env.example
DATABASE_URL=
FEATURE_FLAG=
```

## How to fix

1. Add `KEY=` to the template, with an empty or placeholder value, never the
   real one.
2. Or remove the variable from the env file if nothing uses it.

## How to suppress

Put an inline directive on the line above the variable in the real env file,
or a file-level directive anywhere in it:

```dotenv
# .env
# envguard-ignore EG002 -- personal debugging switch
MY_LOCAL_FLAG=1
```

```dotenv
# envguard-ignore-file EG002
```

In `.envguard.yaml`:

```yaml
version: 1
rules:
  EG002: info   # or warning, or off
ignore:
  - rule: EG002
    path: ".env.local"
    variable: MY_LOCAL_FLAG
    reason: developer-only setting
```

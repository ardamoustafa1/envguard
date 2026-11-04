# EG005 empty-required-value

**Default severity:** warning

A required variable is present but empty in a real env file or the process environment.

## What it detects

- A key whose value is empty (or only whitespace) in a real env file, while
  an applicable template requires it. The template convention is the same as
  for EG001: a variable is required if the template gives it a non-empty
  value, or if its line is not marked `# optional`.
- A schema variable with `required: true` that is empty in an environment
  where it is required.
- When a key is defined more than once, only the last definition (the one
  loaders use) is checked.
- With `--process-env`, empty variables in the process environment.

## Why it matters

An empty value usually counts as "set" and passes presence checks, then fails
later: empty URLs, empty credentials, zero ports.

## Bad example

```dotenv
# .env.example
DATABASE_URL=
PORT=3000
```

```dotenv
# .env
DATABASE_URL=
PORT=3000
```

```text
.env
  1:1  ⚠ warning  EG005  DATABASE_URL is empty in .env but required by .env.example
```

## Good example

```dotenv
# .env
DATABASE_URL=postgres://localhost/app
PORT=3000
```

## How to fix

1. Set a value in the env file named in the finding.
2. If an empty value is intentional and the template value is empty, mark the
   template line optional: `DATABASE_URL= # optional`.

## How to suppress

Put an inline directive on the line above the empty entry in the real env
file, or a file-level directive in it:

```dotenv
# .env
# envguard-ignore EG005 -- filled in by the entrypoint script
DATABASE_URL=
```

```dotenv
# envguard-ignore-file EG005
```

In `.envguard.yaml`:

```yaml
version: 1
rules:
  EG005: info   # or warning, or off
ignore:
  - rule: EG005
    path: ".env.test"
    variable: DATABASE_URL
    reason: tests use an in-memory database
```

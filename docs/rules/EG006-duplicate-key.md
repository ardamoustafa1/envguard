# EG006 duplicate-key

**Default severity:** warning

The same key is defined more than once in one env file or template.

## What it detects

- Every repeated definition of a key in a real env file or template is
  reported at its own line; the other definitions are listed as related
  locations.
- When the values differ, the evidence shows them in masked form. Values of
  sensitive variables are never shown.

## Why it matters

Most loaders (dotenv, python-dotenv, Docker) silently use the last
definition. Editing the first one then has no effect, and the file says two
different things.

## Bad example

```dotenv
# .env
API_URL=http://localhost:8080
LOG_LEVEL=info
API_URL=http://localhost:9090
```

```text
.env
  3:1  ⚠ warning  EG006  API_URL is defined 2 times in .env (lines 1 and 3)
```

## Good example

```dotenv
# .env
API_URL=http://localhost:9090
LOG_LEVEL=info
```

## How to fix

Keep one definition, the one with the value you intend, and delete the
others.

## How to suppress

Put an inline directive on the line above the repeated definition, or a
file-level directive in the file:

```dotenv
# .env
API_URL=http://localhost:8080
# envguard-ignore EG006 -- overrides the line above on purpose
API_URL=http://localhost:9090
```

```dotenv
# envguard-ignore-file EG006
```

In `.envguard.yaml`:

```yaml
version: 1
rules:
  EG006: info   # or warning, or off
ignore:
  - rule: EG006
    path: "legacy/**"
    variable: API_URL
    reason: generated file, the last value wins by design
```

# EG014 invalid-value

**Default severity:** error

A value in a real env file or the process environment violates its schema in `.envguard.yaml`.

## What it detects

The last definition of each key in real env files (or, with
`--process-env`, in the process environment) is validated against the
schema's `variables:` entry. Templates are not validated, since they hold
placeholders, and empty values are EG005's job.

| type | accepts |
|---|---|
| `string` | anything (the default) |
| `int`, `float` | numbers; `min` and `max` apply |
| `bool` | `true/false`, `1/0`, `yes/no`, `on/off`, case-insensitive |
| `url` | a scheme and a host or path; `schemes:` restricts the scheme |
| `port` | 1–65535; `min` and `max` apply |
| `email` | `name@domain.tld` |
| `hostname` | an RFC 1123 hostname, or an IPv4/IPv6 address |
| `duration` | Go syntax: `300ms`, `30s`, `1h30m` |
| `enum` | one of `values` (case-sensitive) |
| `json` | valid JSON |
| `regex` | a valid Go (RE2) regular expression |

Every type also accepts `pattern` (searched; anchor with `^…$` for a full
match), `min_length` and `max_length`. The message states what was
expected. The value appears only in masked form, and never for variables
that are `sensitive: true` or have secret-looking names.

## Why it matters

The app expects a port, a URL, a number or a boolean. An invalid value
usually crashes it at startup or, worse, is silently misread.

## Bad example

```yaml
# .envguard.yaml
version: 1
variables:
  WORKERS: { type: int, min: 1 }
```

```dotenv
# .env
WORKERS=0
```

```text
.env
  2:9  ✗ error    EG014  WORKERS in .env has the invalid value 0: is below the minimum 1
```

## Good example

```dotenv
# .env
WORKERS=4
```

## How to fix

1. Set a value that matches the schema (the fix text describes it, for
   example "an integer of at least 1").
2. If the schema is wrong, update the variable in `.envguard.yaml`.

## How to suppress

Put an inline directive on the line above the entry in the real env file, or
a file-level directive in it:

```dotenv
# .env
# envguard-ignore EG014 -- 0 disables workers in local development
WORKERS=0
```

```dotenv
# envguard-ignore-file EG014
```

In `.envguard.yaml`:

```yaml
version: 1
rules:
  EG014: warning   # or info, or off
ignore:
  - rule: EG014
    path: ".env.development"
    variable: WORKERS
    reason: 0 disables background workers locally
```

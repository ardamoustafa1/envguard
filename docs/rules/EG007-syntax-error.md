# EG007 syntax-error

**Default severity:** error (warning for suspicious but valid syntax)

An env file, Compose file or workflow cannot be parsed, or an env file line cannot be parsed reliably.

## What it detects

- Env files and templates: invalid keys (allowed: letters, digits, `_`, `.`,
  `-`; no leading digit), a missing `=`, unterminated quotes, invalid escape
  sequences in double quotes, and text after a closing quote. These are
  errors.
- Env files and templates: unquoted values containing whitespace, and
  malformed `envguard-ignore` directives. These are warnings.
- Env files that cannot be read, or that are larger than 1 MiB.
- Compose files and GitHub Actions workflows that are not valid YAML.

Messages never repeat the offending text; they name the key (when it is
valid) and the position.

## Why it matters

Env loaders skip or misread malformed lines, so the affected variable ends up
with a wrong value or none at all, often without any error message. An
invalid Compose file or workflow is refused by the tool that reads it.

## Bad example

```dotenv
# .env
1PORT=3000
GREETING=hello world
UNTERMINATED="abc
```

```text
.env
  2:1  ✗ error    EG007  key must not start with a digit
.env
  3:15  ⚠ warning  EG007  GREETING: unquoted value contains whitespace
.env
  4:14  ✗ error    EG007  UNTERMINATED: unterminated double quote
```

## Good example

```dotenv
# .env
PORT=3000
GREETING="hello world"
UNTERMINATED="abc"
```

## How to fix

Every finding includes a specific hint. Typical fixes:

1. Quote values that contain spaces.
2. Close the quote.
3. Use `\\` for a literal backslash in double quotes, or use single quotes.
4. For YAML files, fix indentation, unclosed quotes or brackets, and tabs.

## How to suppress

A directive applies to the next assignment line even when it is malformed,
so it can silence a single problem; a file-level directive covers the file:

```dotenv
# .env
# envguard-ignore EG007 -- parsed by a custom loader
GREETING=hello world
```

```dotenv
# envguard-ignore-file EG007
```

Findings in Compose files and workflows cannot be silenced inline; use
`.envguard.yaml`:

```yaml
version: 1
rules:
  EG007: warning   # or info, or off
ignore:
  - rule: EG007
    path: "fixtures/**"
    reason: intentionally broken test fixtures
```

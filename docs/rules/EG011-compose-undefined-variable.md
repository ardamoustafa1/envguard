# EG011 compose-undefined-variable

**Default severity:** warning

A Compose file interpolates an undefined variable without a default, or a required `env_file` does not exist.

## What it detects

1. A Compose file interpolates `${VAR}` (or `${VAR:?message}`) without a
   default, and nothing defines `VAR`. Compose reads the `.env` file next to
   the Compose file for interpolation. The variable counts as defined if that
   `.env` file, a template in the same directory or in the app root and its
   ancestors, or the schema declares it. If that `.env` does not exist, the
   message says so and only templates and the schema count. One finding per
   Compose file and variable.
2. A required `env_file:` entry points to a file that does not exist.

Not reported: references with a default (`${VAR:-x}`, `${VAR-x}`,
`${VAR:+x}`), `$$` escapes, well-known host variables such as `HOME`,
`scanner.ignore_variables`, and `env_file` entries with `required: false`.

## Why it matters

Compose replaces an unset variable with an empty string and only prints a
warning, so the container starts with a broken value. `${VAR:?message}`
makes `docker compose up` abort, and so does a missing required `env_file`.

## Bad example

```yaml
# docker-compose.yml
services:
  db:
    image: postgres:16
    environment:
      POSTGRES_PASSWORD: ${DB_PASSWORD}
  api:
    env_file: ./api.env
```

```text
docker-compose.yml
  6:28  ⚠ warning  EG011  docker-compose.yml interpolates ${DB_PASSWORD}, which is not defined in .env (the file Compose reads) or any template
docker-compose.yml
  8:15  ⚠ warning  EG011  service api: env_file api.env does not exist
```

## Good example

```yaml
# docker-compose.yml
services:
  db:
    image: postgres:16
    environment:
      POSTGRES_PASSWORD: ${DB_PASSWORD:-postgres}
  api:
    env_file:
      - path: ./api.env
        required: false
```

## How to fix

1. Define the variable in the `.env` next to the Compose file and document it
   in a template (`DB_PASSWORD=` in `.env.example`).
2. Or give it a default: `${DB_PASSWORD:-value}`.
3. For a missing `env_file`, create it (for example from its template) or
   make it optional with the long syntax and `required: false`.

## How to suppress

The finding points at the Compose file, so an inline `# envguard-ignore
EG011` or `# envguard-ignore-file EG011` in an env file does not silence it.
Use `scanner.ignore_variables` for variables the host provides, or
`.envguard.yaml`:

```yaml
version: 1
rules:
  EG011: info   # or warning, or off
ignore:
  - rule: EG011
    path: "deploy/compose.*.yaml"
    variable: IMAGE_TAG
    reason: set by the release pipeline
```

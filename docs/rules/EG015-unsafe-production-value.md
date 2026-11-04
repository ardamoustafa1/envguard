# EG015 unsafe-production-value

**Default severity:** warning

A production value looks like a development setting; active only via schema `production:` rules or a profile.

## What it detects

This rule is **never on by default**. It applies only to real env files of
the `production` environment (`.env.production`, `.env.prod`, or
`--process-env --env production`), skips empty values, and checks the last
definition of each key. It is enabled by either of:

- Per-variable schema rules: `production: { must_be, must_not_be, pattern }`.
  `must_be` and `must_not_be` are compared by type (`off == false`).
- The `production-safety` profile (`profiles: [production-safety]`):

| Variable | Reported when |
|---|---|
| `DEBUG`, `DJANGO_DEBUG`, `APP_DEBUG`, `FLASK_DEBUG` | truthy (`true`, `1`, `yes`, `on`) |
| `NODE_ENV`, `RAILS_ENV` | anything other than `production` |
| `APP_ENV` | `local`, `development` or `dev` |
| `*_URL`, `*_HOST` | points to `localhost`, `127.0.0.1`, `0.0.0.0` or `::1` |

A schema `production:` rule for a variable replaces the profile's check for
it. With `--process-env`, the localhost check applies only to variables
declared in a template or the schema. Localhost URLs are reported without
their value.

## Why it matters

Debug modes expose stack traces and settings. Non-production framework modes
disable caching and security defaults. Inside a container, `localhost` means
the container itself, not your database.

## Bad example

```yaml
# .envguard.yaml
version: 1
profiles: [production-safety]
```

```dotenv
# .env.production
DEBUG=true
DATABASE_URL=postgres://localhost:5432/app
```

```text
.env.production
  2:7  ⚠ warning  EG015  DEBUG is enabled in production (.env.production)
.env.production
  3:14  ⚠ warning  EG015  DATABASE_URL points to a local address in production (.env.production)
```

## Good example

```dotenv
# .env.production
DEBUG=false
DATABASE_URL=postgres://db.internal:5432/app
```

## How to fix

1. Set the production value named in the fix text (`DEBUG=false`,
   `NODE_ENV=production`, the real database host).
2. If a schema rule is wrong, update its `production:` block.

## How to suppress

Put an inline directive on the line above the entry, or a file-level
directive in the production env file:

```dotenv
# .env.production
# envguard-ignore EG015 -- sidecar proxy listens on localhost
DATABASE_URL=postgres://localhost:5432/app
```

```dotenv
# envguard-ignore-file EG015
```

In `.envguard.yaml`:

```yaml
version: 1
rules:
  EG015: info   # or warning, or off
ignore:
  - rule: EG015
    path: ".env.production"
    variable: REDIS_HOST
    reason: Redis runs as a sidecar on localhost
```

# EG013 environment-drift

**Default severity:** warning

Templates of different environments of the same app disagree on which variables exist.

## What it detects

- Only environment-specific templates are compared (`.env.staging.example`
  vs `.env.production.example`), and at least two environments must have
  one.
- Variables in the default template (`.env.example`) apply to every
  environment and are ignored, as are local templates.
- One finding per variable, at its first declaration, listing the
  environments that declare it and the ones that don't; other declarations
  are related locations.

## Why it matters

Environments that run the same code should need the same variables. A
variable documented for only some of them is often forgotten when the next
deploy reaches the others.

## Bad example

```dotenv
# .env.staging.example
API_URL=
```

```dotenv
# .env.production.example
API_URL=
STRIPE_SECRET_KEY=
```

```text
.env.production.example
  2:1  ⚠ warning  EG013  STRIPE_SECRET_KEY is declared for production but not for staging
```

## Good example

```dotenv
# .env.staging.example
API_URL=
STRIPE_SECRET_KEY=
```

## How to fix

1. Add the variable to the other environments' templates.
2. Or move it to `.env.example` if every environment needs it.

## How to suppress

If it really applies to one environment only, put an inline directive on the
line above its declaration, or a file-level directive in the template:

```dotenv
# .env.production.example
# envguard-ignore EG013 -- payments run only in production
STRIPE_SECRET_KEY=
```

```dotenv
# envguard-ignore-file EG013
```

In `.envguard.yaml`:

```yaml
version: 1
rules:
  EG013: info   # or warning, or off
ignore:
  - rule: EG013
    path: ".env.production.example"
    variable: STRIPE_SECRET_KEY
    reason: production-only integration
```

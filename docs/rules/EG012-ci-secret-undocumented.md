# EG012 ci-secret-undocumented

**Default severity:** info

A GitHub Actions workflow references `secrets.X` or `vars.X` that no template or schema documents.

## What it detects

- References are collected from `${{ ... }}` expressions and `if:`
  conditions in `.github/workflows/*.yml` and `*.yaml`.
- A name counts as documented if any template in the repository declares it,
  or `.envguard.yaml` declares it under `variables:`.
- One finding per name, at its first reference, with up to three more
  references as related locations.
- `GITHUB_TOKEN` is provided by GitHub and is never reported; neither are
  names in `scanner.ignore_variables`.

## Why it matters

Whoever sets up a fork, a new environment or a new repository has to
discover the secret by reading workflow files. A missing one usually shows up
as a failed deploy.

## Bad example

```yaml
# .github/workflows/deploy.yml
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: ./deploy.sh
        env:
          DEPLOY_KEY: ${{ secrets.DEPLOY_KEY }}
```

```text
.github/workflows/deploy.yml
  8:27  • info     EG012  workflows use secret DEPLOY_KEY, but no template or schema documents it
```

## Good example

```dotenv
# .env.example
# CI: set in GitHub → Settings → Secrets and variables → Actions (Secrets tab)
DEPLOY_KEY=
```

## How to fix

1. Document it in a template, as above, with a comment saying where to set
   it.
2. Or declare it under `variables:` in `.envguard.yaml`.

## How to suppress

The finding points at a workflow file, so an inline `# envguard-ignore
EG012` or `# envguard-ignore-file EG012` in an env file does not silence it.
Use `.envguard.yaml`:

```yaml
version: 1
rules:
  EG012: off   # or warning, or info
ignore:
  - rule: EG012
    path: ".github/workflows/**"
    variable: CODECOV_TOKEN
    reason: only used by the coverage upload step
```

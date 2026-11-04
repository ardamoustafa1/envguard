# EG008 env-file-committed

**Default severity:** error

A real env file (not a template) is tracked by git.

## What it detects

- Uses `git ls-files` in the scanned root to find real env files
  (`.env`, `.env.production`, `.env.local`, ...) that are tracked.
- Templates (`.env.example`, ...) are meant to be committed and are never
  reported.
- When git is not installed or the root is not a repository, the rule is
  skipped; `check --verbose` says why.

## Why it matters

Everyone with access to the repository, its forks and every clone can read
the values, and they stay in git history even after the file is deleted.

## Bad example

```console
# shell
$ git ls-files
.env.example
.env.production
```

```text
.env.production
  ✗ error    EG008  .env.production is a real env file and is committed to git
```

## Good example

```gitignore
# .gitignore
.env*
!*.example
```

```console
# shell
$ git ls-files
.env.example
.gitignore
```

## How to fix

1. Stop tracking the file and keep your local copy:
   `git rm --cached .env.production`
2. Ignore it, for example with `.env*` and `!*.example` in the root
   `.gitignore`.
3. **Rotate every secret in the file.** The values are still in git history,
   and in every clone and fork. Deleting the file, or even rewriting history,
   does not make them secret again.

## How to suppress

The finding points at the whole file, so a line directive does not apply. A
file-level directive in the env file works:

```dotenv
# .env.ci
# envguard-ignore-file EG008 -- contains only public CI settings
```

In `.envguard.yaml`:

```yaml
version: 1
rules:
  EG008: warning   # or info, or off
ignore:
  - rule: EG008
    path: "e2e/.env.test"
    reason: committed test settings, no secrets
```

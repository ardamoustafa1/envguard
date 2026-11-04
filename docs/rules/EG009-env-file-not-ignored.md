# EG009 env-file-not-ignored

**Default severity:** warning

A real env file is not tracked by git, but no `.gitignore` rule covers it.

## What it detects

- Asks git (`git check-ignore --stdin`, one batched call) whether each
  untracked real env file is ignored.
- Tracked files are EG008's job; templates are never reported.
- The rule is skipped when git is unavailable or the root is not a
  repository.

## Why it matters

The next `git add .` or `git commit -a` commits it, and the values end up in
history, forks and CI logs.

## Bad example

```gitignore
# .gitignore
node_modules/
```

```text
.env
  ⚠ warning  EG009  .env is not ignored by git; `git add .` would commit it
```

## Good example

```gitignore
# .gitignore
node_modules/
.env*
!*.example
```

## How to fix

1. Add the file, or a pattern covering all env files except templates, to
   `.gitignore`.
2. Confirm with `git check-ignore -v .env`.

## How to suppress

The finding points at the whole file, so a line directive does not apply. A
file-level directive in the env file works:

```dotenv
# .env.ci
# envguard-ignore-file EG009 -- meant to be committed, no secrets
```

In `.envguard.yaml`:

```yaml
version: 1
rules:
  EG009: info   # or warning, or off
ignore:
  - rule: EG009
    path: "examples/**"
    reason: demo projects with fake values
```

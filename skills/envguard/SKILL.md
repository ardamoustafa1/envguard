---
name: envguard
description: Check environment variable configuration with EnvGuard. Use for .env templates, env schemas, missing deployment variables, configuration drift, and related Docker Compose or CI changes. Run a check after edits that add, remove, or rename environment variables.
---

Use the installed EnvGuard CLI from the project root, or pass the project's absolute path with `--root`. If `envguard` is not on PATH, check `$HOME/.local/bin/envguard` on Unix. If the binary is unavailable, say so and point to https://github.com/ardamoustafa1/envguard/releases.

- For a check or verification after edits, run `envguard --root "<project>" check --no-color --format pretty`. Add `--env production` or `--app apps/api` when the request targets an environment or app.
- For more context on a finding, run `envguard explain <rule-id>`. For a comparison, use `envguard --root "<project>" diff <from> <to>`.
- A check exits with 0 when the severity threshold passes, 1 when findings meet that threshold, and 2 when the check could not run. Review the report even with exit code 0 because warnings may be present.
- Summarize actionable findings using the rule ID, file, variable name and suggested change. Avoid quoting values or dumping raw env files. Leave live credentials and environment values to the user. Apply template or code fixes only when the task calls for edits, then rerun the check.

# JSON output

`envguard check --format json` and `envguard doctor --format json` print one
JSON document (UTF-8, two-space indentation, trailing newline). `envguard diff
--format json` prints a smaller document described at the end.

The output is **stable**: fields are only ever added, never renamed, removed or
given a new meaning, without bumping `schema_version`. Consumers should ignore
fields they do not know. Arrays are always present (`[]` when empty), never
`null`. Output is deterministic: the same input gives byte-identical JSON (it
contains no timestamps or durations).

**Values are never included.** Messages, `why` and `fix` describe problems
without values, `evidence` is produced by EnvGuard's masking code (for example
`detected: AWS access key ID` or `•••••• (32 chars)`), and the matrix records
presence only.

## Check and doctor

```json
{
  "schema_version": 1,
  "tool": { "name": "envguard", "version": "1.2.0" },
  "command": "check",
  "root": ".",
  "process_env": false,
  "stats": { ... },
  "summary": { "errors": 2, "warnings": 3, "info": 1, "suppressed": 0 },
  "findings": [ ... ],
  "matrix": [ ... ],
  "ci": { "secrets": [ ... ], "vars": [ ... ] },
  "notes": [ ... ]
}
```

| Field | Type | Meaning |
|---|---|---|
| `schema_version` | int | Version of this format. Currently `1`. |
| `tool.name` / `tool.version` | string | Always `envguard` / the binary version (`dev` for local builds). |
| `command` | string | `check` or `doctor`. |
| `root` | string | The `--root` argument as given, slash-separated. All paths below are relative to it. |
| `process_env` | bool | `true` with `check --process-env`. |
| `stats` | object | What was analyzed, see below. |
| `summary` | object | Finding counts by severity after suppression, plus `suppressed` (hidden by config ignores or `envguard-ignore` directives). |
| `findings` | array | See below. Sorted by file, line, column, rule ID. |
| `matrix` | array | The environment matrix per app root, see below. |
| `ci` | object | Secrets and vars that GitHub Actions workflows reference. |
| `notes` | array | Informational scanner remarks (e.g. a server-only variable read in Next.js client code). |

### `stats`

| Field | Type | Meaning |
|---|---|---|
| `env_files` | int | Real env files (`.env`, `.env.production`, ...). |
| `templates` | int | Templates (`.env.example`, ...). |
| `compose_files` | int | Docker Compose files. |
| `workflows` | int | GitHub Actions workflows. |
| `source_files` | int | Source files the scanner read (0 with `--process-env`). |
| `languages` | array | `{ "language": "javascript", "files": 12 }`, most files first. |
| `variables` | int | Distinct variable names across env files, templates, code, Compose, workflows and the schema. |
| `app_roots` | array of string | App roots (directories with `package.json`, `go.mod`, ...); always contains `.`. |
| `dynamic_lookups` | int | Computed-name reads such as `process.env[name]` that could not be analyzed. |

### `findings[]`

| Field | Type | Meaning |
|---|---|---|
| `rule_id` | string | `EG001` ... `EG016`. IDs are permanent. |
| `rule_name` | string | `missing-variable`, ... |
| `severity` | string | `error`, `warning` or `info` (after `rules:` overrides). |
| `message` | string | One line. |
| `variable` | string | The variable concerned, or `""`. |
| `location` | object | `{ "file", "line", "column" }`. `line`/`column` are 1-based; `0` means the whole file. Columns count Unicode code points. The process environment is the file `(process environment)`. |
| `evidence` | string | Optional masked detail, or `""`. |
| `why` | string | Why it matters. |
| `fix` | string | How to fix it. May contain newlines, numbered steps (`1. ...`) and indented code blocks. |
| `related` | array | Other relevant locations (e.g. where a variable is declared), each `{ "file", "line", "column" }` like `location`. |
| `docs_url` | string | Rule documentation. |

### `matrix[]`

One entry per app root that has env files or templates:

```json
{
  "app_root": ".",
  "environments": ["default", "production"],
  "template_only_environments": ["staging"],
  "files": { "default": [".env"], "production": [".env.production"] },
  "variables": [
    { "name": "DATABASE_URL", "cells": { "default": "set", "production": "missing" } }
  ]
}
```

`environments` are the environments with at least one real env file (columns;
with `--process-env`, the process environment's environment). Environments
that only have templates are listed in `template_only_environments` and have
no cells. `files` maps each column to the real env files that feed it, in
order. The `--env` filter limits the columns. Every row has one cell per
column:

| Cell | Meaning |
|---|---|
| `set` | A real env file of the environment sets a non-empty value (last definition wins). |
| `empty` | Defined, but empty. |
| `missing` | A template that applies to the environment requires it (not marked `# optional`, or given a non-empty value), but no real env file defines it. |
| `absent` | Not defined, and not required for that environment (not declared, or declared `# optional`). |

Rows are the variables of the app's real env files and of the templates that
apply to its columns (only template variables with `--process-env`), sorted
by name. In an environment whose files do not define a variable, its cell is
`missing` or `absent`.

### `ci`

```json
{ "secrets": [ { "name": "DEPLOY_TOKEN", "refs": [ { "file": ".github/workflows/deploy.yml", "line": 14, "column": 30 } ] } ], "vars": [] }
```

`GITHUB_TOKEN` is never listed.

### `notes[]`

`{ "location": { ... }, "variable": "API_SECRET", "text": "Next.js only inlines NEXT_PUBLIC_* ..." }`

## Diff

`envguard diff --format json` (no arguments):

```json
{ "schema_version": 1, "tool": { ... }, "command": "diff", "root": ".", "matrix": [ ... ] }
```

`matrix` has the shape described above.

`envguard diff staging production --format json`:

```json
{
  "schema_version": 1, "tool": { ... }, "command": "diff", "root": ".",
  "diff": {
    "a": "staging",
    "b": "production",
    "apps": [
      {
        "app_root": ".",
        "a_files": [".env.example", ".env.staging.example"],
        "b_files": [".env.production"],
        "a_from_templates": true,
        "b_from_templates": false,
        "only_in_a": ["STAGING_BANNER"],
        "only_in_b": [],
        "common": 8
      }
    ]
  }
}
```

`a`/`b` are canonical environment names (aliases such as `prod` are
resolved). A side without a real env file is represented by its templates
(`*_from_templates: true`). Only app roots where at least one side has files
are listed.

## Exit codes

JSON output does not change exit codes: `0` no findings at or above `--fail-on`,
`1` findings at or above it (or differences with `diff --exit-code`), `2`
configuration or runtime error (then stdout is empty and stderr has the error).

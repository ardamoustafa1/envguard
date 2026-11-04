# Rule documentation

One file per rule, named `EGxxx-<rule-name>.md` (for example
`EG001-missing-variable.md`). These files are embedded into the binary by
package `docs` and are the single source of truth for `envguard explain` and
the website.

Each rule document has the same structure, with these headings verbatim and
in this order (tools parse it):

1. `# EGxxx rule-name` title.
2. `**Default severity:** ...` line.
3. A one-sentence summary on a single line (used by `envguard explain --list`
   and as the SARIF short description).
4. `## What it detects` — the precise trigger conditions.
5. `## Why it matters` — 1-3 sentences.
6. `## Bad example` — a failing example (no real-looking secrets), optionally
   followed by the finding as EnvGuard prints it.
7. `## Good example` — the fixed version.
8. `## How to fix` — concrete steps.
9. `## How to suppress` — inline and file-level directives (where they apply)
   and `.envguard.yaml` severity overrides and ignores.

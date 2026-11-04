# Contributing to EnvGuard

Thank you for helping. This file covers setup, the rules of the project and
two step-by-step guides: adding a rule and adding a language detector.
[CLAUDE.md](CLAUDE.md) is the deep reference for architecture and decisions.
Read the relevant parts before a larger change.

## Development setup

You need Go 1.26 or newer (the minimum in `go.mod`) and `git`. `node` is only
needed for `make snapshot`.

```sh
git clone https://github.com/ardamoustafa1/envguard
cd envguard
make build     # bin/envguard
make test      # go test -race ./...
make lint      # golangci-lint, installed into bin/ on first use
```

| Command | What it does |
|---|---|
| `make test` | All tests with the race detector. |
| `make lint` | `golangci-lint run ./...` with the pinned version. |
| `make fuzz` | Every `Fuzz*` target for `FUZZTIME` each (default 30s). Example: `make fuzz FUZZTIME=2m`. |
| `make cover` | Coverage profile and `coverage.html`. |
| `make snapshot` | Local release dry run with GoReleaser. Publishes nothing. |

### Golden files

Many tests compare output with files in a `testdata/` directory. Update them
**one package at a time**, and review the diff before you commit:

```sh
go test ./internal/parser -run TestGolden -update
go test ./internal/rules -run TestRuleFixtures -update
go test ./internal/scanner -run TestFixtureProjects -update
go test ./internal/report -update
go test ./cmd/envguard -run TestCheckGolden -update
```

Never run `go test ./... -update`. Packages that do not import
`internal/testutil` do not define the flag, so the run fails.

## Project layout

```
cmd/envguard        cobra commands; the only place that maps errors to exit codes
internal/discovery  walks the repo, classifies files
internal/parser     dotenv parser with exact positions
internal/scanner    finds env var reads in source code
internal/compose    Docker Compose parsing
internal/workflow   GitHub Actions parsing
internal/project    one immutable Project model built from all inputs
internal/rules      the rules; pure functions of Project
internal/mask       the only place that turns a value into output text
internal/report     pretty, doctor, json, sarif, github output
internal/testutil   test helpers (golden files, fixtures, fake secrets)
docs/rules          one document per rule, embedded into the binary
testdata/fixtures   positive and negative fixtures for each rule
```

## Principles

A change that breaks one of these is a bug.

- **Offline.** No network code, ever. `TestOffline` checks the dependency graph.
- **Read-only.** EnvGuard does not modify user files. The few exceptions
  (`init`, `--output`, the job summary) are listed in CLAUDE.md.
- **Never print values.** Value text reaches output only through
  `internal/mask`. `TestLeakCanary` runs every command on a repo full of
  `CNRY` canary values and fails if one appears. Never weaken that test.
- **Deterministic.** Same input, same bytes. Never let map order reach output.
- **Low false positives beat more rules.** When unsure, use `info` or emit
  nothing. A missed finding is acceptable. A wrong one is not.
- Every finding has a `Why` and a `Fix`.

## How to add a new rule

Rule IDs are permanent. Never renumber or reuse one. The examples use a
made-up `EG017 example-rule`.

1. **Pick the next free ID.** Look at `internal/rules/` and the table in
   CLAUDE.md section 5. Choose a short kebab-case name.

2. **Create `internal/rules/eg017_example_rule.go`.** Implement the `Rule`
   interface (`ID`, `Name`, `DefaultSeverity`, `Check`) by embedding `base`,
   and register in `init`. This skeleton follows `eg006_duplicate_key.go`:

   ```go
   package rules

   import (
   	"fmt"

   	"github.com/ardamoustafa1/envguard/internal/findings"
   	"github.com/ardamoustafa1/envguard/internal/project"
   )

   func init() { Register(exampleRule{base{"EG017", "example-rule", findings.Warning}}) }

   // exampleRule reports ... (one sentence: the exact trigger).
   type exampleRule struct{ base }

   func (exampleRule) Check(p *project.Project) []findings.Finding {
   	var out []findings.Finding
   	for _, f := range p.EnvFiles {
   		if !p.EnvSelected(f.Environment) {
   			continue
   		}
   		for _, e := range f.Entries() {
   			if !problem(e) { // your condition
   				continue
   			}
   			out = append(out, findings.Finding{
   				Message:  fmt.Sprintf("%s in %s ...", e.Key, f.Path),
   				Variable: e.Key,
   				File:     f.Path,
   				Line:     e.KeyPos.Line,
   				Column:   e.KeyPos.Column,
   				Why:      "One to three sentences: what breaks and why it matters.",
   				Fix:      fmt.Sprintf("Concrete steps naming %s and the line.", f.Path),
   			})
   		}
   	}
   	return out
   }
   ```

   The engine fills `RuleID`, `RuleName`, `DocsURL` and the default severity,
   applies `rules` overrides and suppression, and sorts the result.

3. **Follow the finding contract.**
   - `Message`, `Why` and `Fix` are never empty. The engine rejects the
     finding otherwise (`TestRunRejectsFindingsWithoutWhyOrFix`).
   - Never put a value in `Message`, `Why`, `Fix` or `Evidence`.
     `TestNoValuesInMessages` checks it. If a value helps, pass it through
     `p.DisplayValue(name, value)`. It masks secrets, sensitive names and
     schema `sensitive: true`.
   - A rule is a pure function of `*project.Project`: no I/O, no globals, no
     time, no randomness.
   - Inline `envguard-ignore` directives and config ignores are applied by the
     engine. Do not handle them in the rule. Reuse the helpers in
     `internal/rules/helpers.go` and name exact files and lines in `Fix`.
   - Zero config must work. Use `info` or stay silent when unsure.

4. **Add fixtures** in `testdata/fixtures/EG017/`:
   - `positive/` must produce at least one finding. `negative/` must produce
     none from this rule. Each is usually its own app root, so add a
     `package.json` containing `{}` to each.
   - Findings print with the directory in the path, for example
     `EG017/positive/.env`.
   - Nested `.gitignore` files are stored as `_gitignore`. The test copy
     renames them (see `testdata/fixtures/EG008/negative/_gitignore`).
   - Rules that need git state add `testdata/fixtures/EG017/git-tracked.txt`,
     one path per line (see `testdata/fixtures/EG008/git-tracked.txt`). The
     test runs `git init` and `git add -f` on a copy.
   - Never write a real-looking secret. In a static file write
     `{{secret:github}}` (kinds: aws, github, slack, stripe, openai,
     anthropic, google, sendgrid, twilio, pem, jwt, url, entropy). In Go code
     use `testutil.FakeSecret(kind, seed, marker)`.
   - Fixture file names must be portable: no case-only differences and no
     trailing dots.

5. **Generate the rule golden.**

   ```sh
   go test ./internal/rules -run TestRuleFixtures -update
   ```

   This writes `testdata/fixtures/EG017/golden.txt` with every field,
   including the `Why` and `Fix` wording. Read it. Then run
   `go test ./internal/rules` without `-update`. `TestRegistry` fails if the
   rule has no document.

6. **Write `docs/rules/EG017-example-rule.md`.** Use exactly this structure.
   `TestEveryRuleHasDocs` (`internal/report`) enforces the headings, the
   severity line, a summary ending in a period and the file name:

   ```markdown
   # EG017 example-rule

   **Default severity:** warning

   One sentence summary that ends with a period.

   ## What it detects
   ## Why it matters
   ## Bad example
   ## Good example
   ## How to fix
   ## How to suppress
   ```

   Show the finding in the pretty format (a path line, then
   `  3:1  ⚠ warning  EG017  message`). Copy `docs/rules/EG006-duplicate-key.md`
   for the suppression section. See also `docs/rules/README.md`.

7. **Regenerate the other goldens**, one package at a time, and review each diff:

   ```sh
   go test ./cmd/envguard -run TestCheckGolden -update
   go test ./internal/report -update
   go test ./cmd/envguard -run TestReadmeRulesTable -update
   ```

   The first rewrites `cmd/envguard/testdata/check.golden` (every fixture
   goes through `check`). The second rewrites the report goldens, including
   `explain-list.golden` and SARIF. The third refreshes the rules table in
   `README.md`. If the new rule changes the output of existing fixtures, check
   that the change is expected.

8. **Extend the leak canary** in `cmd/envguard/leak_canary_test.go` if the
   rule adds an output path or reads values in a new place. Add a
   canary-valued input to `canaryRepo` that would trigger the rule. Run
   `go test ./cmd/envguard -run TestLeakCanary`.

9. **Update CLAUDE.md.** Add the rule to the catalog in section 5 (and its
   conventions if it has any) and to the implemented list.

10. Run `make test` and `make lint`.

## How to add a language detector

The scanner prefers precision over recall. A missed usage is acceptable.
A phantom usage is not. The examples use a made-up language `Lua`.

1. **Register the extension.** In `internal/discovery/types.go` add a
   `Language` constant (for example `LangLua Language = "lua"`) and the
   extension to `extLanguages`:

   ```go
   ".lua": LangLua,
   ```

2. **Describe the syntax and detectors** in `internal/scanner/detectors.go`.
   A `syntax` tells the lexer what is a comment and a string. A match counts
   only if it starts in code, so usages in comments, strings and regex
   literals are ignored. Copy the closest language, for example:

   ```go
   var luaSyntax = &syntax{
   	line:  []string{"--"},
   	block: []blockComment{{open: "--[[", close: "]]"}},
   	quotes: []quote{
   		{open: `"`, close: `"`, escape: true},
   		{open: `'`, close: `'`, escape: true},
   	},
   }

   var luaDetectors = []*detector{
   	newDetector(detector{pattern: `os.getenv`, hint: "getenv",
   		re: regexp.MustCompile(`\bos\.getenv\(\s*` + quotedName + `\s*\)`),
   		after: regexp.MustCompile(`^\s*or\b`)}),
   	newDetector(detector{pattern: "os.getenv(expr)", hint: "os.",
   		re: regexp.MustCompile(`\bos\.getenv\(\s*` + notQuoteOrEnd)}),
   }
   ```

   Then add it to the `languages` map:

   ```go
   discovery.LangLua: {syntax: luaSyntax, detectors: luaDetectors},
   ```

   Each detector regex needs a `name` group (the variable) or a `dyn` group
   (a computed key, reported as Dynamic). Optional fields:

   | Field | Use |
   |---|---|
   | `hint` | A literal that must occur in the file. Cheap pre-filter. |
   | `requires` | File-level gate, for example an import. |
   | `after` | Matched after the usage. A match means a default follows (`\|\|`, `??`, `.unwrap_or`). |
   | `skipAfter` | A match means it is not a read (assignment, method call). |
   | `alwaysDefault` | The API tolerates absence (`LookupEnv`, `option_env!`). |
   | `upperOnly` | Property-style access accepts only UPPER_CASE names. |
   | `skipNames`, `skipPrefixes` | Built-ins that are not env reads. |
   | `def` regex group | A non-empty match means a default argument. |

   Always add the dynamic form, so computed keys are counted, not guessed.

3. **Check the lexer.** If the language has syntax the lexer lacks (nested
   comments, raw strings, heredocs, interpolation), see the flags in
   `internal/scanner/lexer.go` (`jsTemplate`, `rustRaw`, `keepCode`, ...) and
   how `rustSyntax` and `shellSyntax` use them. If you cannot handle a
   construct exactly, do not guess. Leave it out and say so in the docs.
   Embedded languages use `regions` (Vue, PHP).

4. **Add table tests** in `internal/scanner/scanner_test.go`, following
   `TestGo` and `TestJavaScript`. Each case is `{name, source, want}` where
   `want` lists `NAME@line:col`, with `+d` for a default and `dyn@line:col`
   for dynamic usages. Cover: each API, defaults, dynamic keys, comments,
   strings, unicode columns, and names that look like the API but are not.
   Add the language to `TestNeverPanics` (it iterates `languages`, so it is
   covered automatically).

5. **Add a fixture project** in `internal/scanner/testdata/scanner/lua/` with a
   marker file for the app root (for example a `package.json` or the
   language's own manifest). The convention: names containing `FAKE` sit in
   comments, strings or assignments and must never be reported. Every
   `REAL_*` token must be reported, except `REAL_DYNAMIC`, which appears only
   as a computed key. Then regenerate the golden:

   ```sh
   go test ./internal/scanner -run TestFixtureProjects -update
   ```

   Review the diff in `internal/scanner/testdata/scanner.golden.json`.

6. **Fuzz.** The dotenv parser has `FuzzParse` in `internal/parser`. No scanner
   fuzz target exists yet. Adding one is welcome. A fuzz harness must be
   linear-time itself. `make fuzz` runs all targets. Also run
   `go test ./internal/scanner -bench Scan` to check the speed did not regress.

7. **Update the docs.** Add the language to the language table in
   `docs/how-it-works.md`, the scanner section of CLAUDE.md, and
   `CHANGELOG.md`. List what is deliberately unsupported.

8. Run `make test` and `make lint`.

## Report a false positive

Use the "False positive" issue template. Give the rule ID, the finding line as
printed, a minimal snippet and `envguard version`. **Never paste real secrets
or real values.** Replace them with fakes. A reproduction that needs a real
secret is not needed. Fixing false positives has priority over new rules.

## Commits and pull requests

- Keep a PR focused on one change.
- Write the commit subject in the imperative: "Add EG017 example-rule".
  Explain the why in the body if it is not obvious.
- Fill in the pull request template. Its checklist is the definition of done
  from CLAUDE.md.
- There is no DCO and no CLA. You keep your copyright and license your work
  under the project license by contributing it.
- Be kind. See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## Good first issues

Ready-to-use drafts (new languages, framework notes, secret patterns) are in
[docs/roadmap/good-first-issues.md](docs/roadmap/good-first-issues.md). The
plan is in [docs/roadmap.md](docs/roadmap.md).

## Security

Report vulnerabilities privately. See [SECURITY.md](SECURITY.md).

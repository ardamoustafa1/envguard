# Good first issues

Ready-to-paste issue drafts. Copy a block into a new issue, add the labels.

Before you start, read [CONTRIBUTING.md](../../CONTRIBUTING.md). New detectors
follow "How to add a language detector". Fixtures must never contain
real-looking secrets. Build fake tokens at runtime with `testutil.FakeSecret`
or write `{{secret:KIND}}` in static fixture files. A new kind needs a case in
`internal/testutil/fixtures.go` and an entry in `SecretKinds`.

Already supported, so not listed: Deno.env.get, Bun.env, Next.js and Vite
notes, and the secret formats in `internal/mask/secret.go` (AWS, GitHub, Slack,
Stripe, Anthropic, OpenAI, Google, SendGrid, Twilio, PEM, JWT).

## 1. Detect environment variables in Dart and Flutter

Labels: `new-language`, `good first issue`

Context: Dart reads variables with `Platform.environment['X']` and
`String.fromEnvironment('X', defaultValue: 'd')`. EnvGuard does not scan
`.dart` files.

Acceptance criteria:
- `.dart` maps to a new `Language` and is scanned.
- Detects `Platform.environment['X']` (with `?? 'd'` as a default) and
  `String.fromEnvironment('X')` / `bool.fromEnvironment` / `int.fromEnvironment`
  (`defaultValue:` is a default).
- Computed keys are Dynamic. Comments (`//`, `/* */`, `///`) and strings are ignored.
- Table test, fixture project with `FAKE_*` and `REAL_*` names, updated goldens.

Pointers: `internal/discovery/types.go` (`extLanguages`),
`internal/scanner/detectors.go` (`javaSyntax` is a close model),
`internal/scanner/scanner_test.go`, `internal/scanner/testdata/scanner/`.

## 2. Detect environment variables in Swift

Labels: `new-language`, `good first issue`

Context: Swift uses `ProcessInfo.processInfo.environment["X"]`. Not scanned today.

Acceptance criteria:
- `.swift` is scanned.
- Detects `ProcessInfo.processInfo.environment["X"]`, with `?? "d"` as a default,
  and `getenv("X")`.
- Handles `//`, nested `/* */`, `"""` multi-line strings and `#"raw"#` strings
  (an unsupported raw string may be listed as a known limit).
- Tests and fixture as in the guide.

Pointers: `internal/scanner/detectors.go` (`rustSyntax` for nested comments),
`internal/scanner/lexer.go`.

## 3. Detect environment variables in Scala

Labels: `new-language`, `good first issue`

Context: Scala reads `sys.env("X")`, `sys.env.get("X")`, `sys.env.getOrElse("X", "d")`
and `System.getenv("X")`.

Acceptance criteria:
- `.scala` is scanned.
- `sys.env.get` and `getOrElse` count as defaults (`alwaysDefault`).
- Scala string interpolation (`s"..."`) is not analyzed, same as Kotlin.
- Tests and fixture as in the guide.

Pointers: `javaDetectors` in `internal/scanner/detectors.go`
(`System.getenv` already exists for Java and Kotlin).

## 4. Framework note for Astro

Labels: `framework`, `good first issue`

Context: Astro exposes only `PUBLIC_*` variables to client code through
`import.meta.env`. The scanner adds notes for Next.js (`NEXT_PUBLIC_`) and Vite
(`VITE_`) but not Astro. Astro is built on Vite, so today a plain Vite note may
apply when both are listed.

Acceptance criteria:
- `astro` in `package.json` is detected as a framework.
- Reads of names other than `PUBLIC_*` in files under the app's `src/` get an
  info note, as for Vite. Astro wins over Vite when both are listed.
- `.astro` files are out of scope unless a detector is added.
- Test in `TestFrameworkNotes`.

Pointers: `internal/scanner/framework.go`, `internal/scanner/fixtures_test.go`.

## 5. Framework note for SvelteKit

Labels: `framework`, `good first issue`

Context: SvelteKit reads variables through `$env/static/public`,
`$env/static/private`, `$env/dynamic/public` and `$env/dynamic/private`. Public
names must start with `PUBLIC_`. These imports are not detected as usages.

Acceptance criteria:
- `import { PUBLIC_X } from '$env/static/public'` reports `PUBLIC_X`.
- `import { X } from '$env/static/private'` reports `X`.
- `env.X` from `$env/dynamic/*` is detected only when the import is present
  (`requires`).
- Tests and a fixture.

Pointers: `jsDestructuring` extractor and `requires` in
`internal/scanner/detectors.go`.

## 6. Framework note for Create React App

Labels: `framework`, `good first issue`

Context: Create React App inlines only `REACT_APP_*` (and `NODE_ENV`,
`PUBLIC_URL`) into the browser bundle. A read of another name is `undefined`
in the browser.

Acceptance criteria:
- `react-scripts` in `package.json` is detected as a framework.
- Client files under `src/` get an info note for names outside `REACT_APP_*`,
  `NODE_ENV` and `PUBLIC_URL`.
- Test in `TestFrameworkNotes`.

Pointers: `internal/scanner/framework.go` (`readFramework`, the Vite branch).

## 7. Framework note for Nuxt

Labels: `framework`, `good first issue`

Context: Nuxt reads configuration through `runtimeConfig`. Variables named
`NUXT_PUBLIC_*` override public runtime config. `process.env` in client code
is not populated at runtime.

Acceptance criteria:
- `nuxt` in `package.json` is detected.
- A note explains that `process.env.X` in a client file (under `components/`,
  `pages/`, `composables/`) is not available and suggests `runtimeConfig`.
  The wording stays "heuristic, info only".
- Test and fixture.

Pointers: `internal/scanner/framework.go`. Nuxt `.vue` files are scanned
already.

## 8. Detect GitLab tokens (glpat-)

Labels: `secret-pattern`, `good first issue`

Context: `mask.LooksLikeSecret` does not know GitLab personal access tokens
(`glpat-` followed by 20 URL-safe characters). A committed token in a template
should raise EG010.

Acceptance criteria:
- New entry in `secretPatterns` in `internal/mask/secret.go`, kind
  "GitLab personal access token", placed before the generic patterns.
- Positive cases in `TestLooksLikeSecret`, near-miss negatives in `TestNotSecrets`.
- Fixtures build the token at runtime: add a `gitlab` kind to
  `testutil.FakeSecret` (and `SecretKinds`), or concatenate strings. Never
  commit a token-shaped literal. `TestFakeSecretsAreDetected` covers new kinds.
- `CLAUDE.md` mask section and the EG010 doc mention the new format.

Pointers: `internal/mask/secret.go`, `internal/mask/secret_test.go`,
`internal/testutil/fixtures.go`.

## 9. Detect npm tokens (npm_)

Labels: `secret-pattern`, `good first issue`

Context: npm automation and publish tokens look like `npm_` plus 36
alphanumeric characters. They are not detected.

Acceptance criteria:
- New pattern, kind "npm access token".
- Tests and a `FakeSecret` kind as in issue 8, no literals in the repository.
- Short or placeholder values such as `npm_` followed by `xxxx` are not flagged.

Pointers: same files as issue 8.

## 10. Detect Hugging Face tokens (hf_)

Labels: `secret-pattern`, `good first issue`

Context: Hugging Face user tokens look like `hf_` plus 34 letters. They are
not detected. The prefix is short, so the pattern needs a word boundary and a
fixed length to avoid false positives.

Acceptance criteria:
- New pattern, kind "Hugging Face access token", with a word boundary and a
  length check.
- Negative tests: a variable name or word that merely starts with `hf_`.
- `FakeSecret` kind as in issue 8, no literals.

Pointers: same files as issue 8. A DigitalOcean (`dop_v1_` plus 64 hex
characters) pattern is a good follow-up with the same approach.

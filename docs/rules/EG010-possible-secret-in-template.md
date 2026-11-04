# EG010 possible-secret-in-template

**Default severity:** error

A template, or a real env file that git tracks, contains a value that looks like a real credential.

## What it detects

Every entry of every template, and of every git-tracked real env file, is
checked against known credential formats: AWS access key IDs; GitHub tokens
(`ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_`, `github_pat_`); Slack tokens and
webhooks; Stripe live keys (test keys do not count); OpenAI- and
Anthropic-style `sk-` keys; Google `AIza` keys; SendGrid keys; Twilio API
keys; PEM private keys; JWTs; URLs with an embedded password.

- Under sensitive names (`*_SECRET`, `*_TOKEN`, `*_KEY`, ...), a value of 20
  or more characters without spaces and with Shannon entropy of at least 3.5
  also counts.
- Obvious placeholders never count: empty values, `changeme`, `your-...`,
  `<...>`, `xxxx...`, `***`, `example`, `dummy`, `test`, `placeholder`,
  `replace-me`, `todo`, the key name itself, localhost URLs and local
  development credentials.
- The message names the detected kind; the value is never printed.

## Why it matters

Templates are committed and shared with everyone who can read the
repository. A real credential there is leaked, and it stays in git history.

## Bad example

```dotenv
# .env.example
GITHUB_TOKEN=ghp_<the rest of a real 36-character token>
DATABASE_URL=postgres://app:<a real password>@db.example.com/app
```

```text
.env.example
  2:14  ✗ error    EG010  GITHUB_TOKEN in .env.example looks like a GitHub personal access token
```

## Good example

```dotenv
# .env.example
GITHUB_TOKEN=
DATABASE_URL=postgres://user:password@localhost:5432/app
```

## How to fix

1. Replace the value with an empty one or an obvious placeholder
   (`API_KEY=<your-api-key>`).
2. If it was a real credential, rotate it: it has been exposed, and it stays
   in git history.

## How to suppress

Prefer an obvious placeholder. For a deliberate fake, put an inline
directive on the line above it, or a file-level directive in the file:

```dotenv
# .env.example
# envguard-ignore EG010 -- documented demo token, revoked
DEMO_TOKEN=<demo token>
```

```dotenv
# envguard-ignore-file EG010
```

In `.envguard.yaml`:

```yaml
version: 1
rules:
  EG010: warning   # or info, or off
ignore:
  - rule: EG010
    path: "docs/examples/**"
    variable: DEMO_TOKEN
    reason: revoked token used in documentation
```

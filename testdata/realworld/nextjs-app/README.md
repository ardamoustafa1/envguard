# Acme SaaS

Next.js 15 (App Router) + Prisma + Auth.js + Stripe.

## Getting started

```bash
cp .env.example .env.local
docker compose up -d db
npm install
npm run db:migrate
npm run dev
```

Open http://localhost:3000.

## Environment variables

All variables are validated at startup in `lib/env.ts`. See `.env.example`
for the full list. Set `SKIP_ENV_VALIDATION=1` to skip validation (used in the
Docker build).

| Variable | Description |
| --- | --- |
| `DATABASE_URL` | Postgres connection string |
| `NEXTAUTH_SECRET` | Session signing secret |
| `NEXTAUTH_URL` | Canonical URL of the app |
| `AUTH_GITHUB_ID` / `AUTH_GITHUB_SECRET` | GitHub OAuth app credentials |
| `STRIPE_SECRET_KEY` / `STRIPE_WEBHOOK_SECRET` | Stripe API credentials |
| `NEXT_PUBLIC_APP_URL` | Public base URL |
| `NEXT_PUBLIC_POSTHOG_KEY` | Optional analytics key |

## Testing

```bash
npm test          # unit tests (vitest)
npm run test:e2e  # Playwright
```

## Deployment

The `Dockerfile` produces a standalone build. Run migrations with
`npm run db:deploy` before starting a new release.

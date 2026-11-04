# Acme Platform

pnpm + Turborepo monorepo.

| Path | What |
| --- | --- |
| `apps/web` | Next.js 14 front end |
| `apps/api` | Fastify + TypeScript HTTP API |
| `apps/worker` | BullMQ background worker |
| `packages/config` | zod schema for the process environment, shared by the apps |
| `packages/db` | Prisma schema and client |
| `packages/ui` | React component library |

## Development

```bash
pnpm install
docker compose up -d
cp .env.example .env
cp apps/api/.env.example apps/api/.env
cp apps/web/.env.example apps/web/.env.local
cp apps/worker/.env.example apps/worker/.env
pnpm db:migrate
pnpm dev
```

## Environment

Every variable is documented in the relevant `.env.example`. Server-side code must read the environment through
`@acme/config`, which validates it with zod at start-up.

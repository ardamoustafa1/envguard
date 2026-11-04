FROM node:20-alpine AS base
RUN corepack enable
WORKDIR /repo

FROM base AS build
COPY . .
RUN pnpm install --frozen-lockfile
RUN pnpm --filter @acme/worker... build
RUN pnpm deploy --filter @acme/worker --prod /out

FROM base AS runner
ENV NODE_ENV=production
WORKDIR /app
COPY --from=build /out .
CMD ["node", "dist/index.js"]

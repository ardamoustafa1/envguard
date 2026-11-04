FROM node:20-alpine AS base
RUN corepack enable
WORKDIR /repo

FROM base AS build
COPY . .
RUN pnpm install --frozen-lockfile
RUN pnpm --filter @acme/api... build
RUN pnpm deploy --filter @acme/api --prod /out

FROM base AS runner
ENV NODE_ENV=production
WORKDIR /app
COPY --from=build /out .
EXPOSE 4000
CMD ["node", "dist/server.js"]

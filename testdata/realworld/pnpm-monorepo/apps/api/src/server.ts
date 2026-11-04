import Fastify from "fastify";
import cors from "@fastify/cors";
import jwt from "@fastify/jwt";
import { config, isProduction } from "./config.js";
import { healthRoutes } from "./routes/health.js";
import { userRoutes } from "./routes/users.js";

export async function buildServer() {
  const app = Fastify({
    logger: {
      level: config.LOG_LEVEL,
      transport: isProduction ? undefined : { target: "pino-pretty" },
    },
  });

  await app.register(cors, { origin: config.CORS_ORIGIN.split(",") });
  await app.register(jwt, { secret: config.JWT_SECRET });
  await app.register(healthRoutes);
  await app.register(userRoutes, { prefix: "/users" });

  return app;
}

if (process.env.VITEST === undefined) {
  const app = await buildServer();
  await app.listen({ port: config.PORT, host: config.HOST });
}

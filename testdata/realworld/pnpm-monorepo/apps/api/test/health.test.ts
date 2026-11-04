import { beforeAll, describe, expect, it } from "vitest";

beforeAll(() => {
  process.env.DATABASE_URL = "postgres://postgres:postgres@localhost:5432/test";
  process.env.REDIS_URL = "redis://localhost:6379";
  process.env.JWT_SECRET = "test-secret";
  process.env.CORS_ORIGIN = "http://localhost:3000";
});

describe("GET /health", () => {
  it("responds with ok", async () => {
    const { buildServer } = await import("../src/server.js");
    const app = await buildServer();
    const res = await app.inject({ method: "GET", url: "/health" });
    expect(res.statusCode).toBe(200);
    expect(res.json().status).toBe("ok");
  });
});

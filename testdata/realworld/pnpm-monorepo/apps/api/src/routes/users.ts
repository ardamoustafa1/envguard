import type { FastifyInstance } from "fastify";
import { prisma } from "@acme/db";

export async function userRoutes(app: FastifyInstance) {
  app.get("/", async () => {
    return prisma.user.findMany({ orderBy: { createdAt: "desc" }, take: 100 });
  });

  app.post<{ Body: { email: string } }>("/", async (req, reply) => {
    const user = await prisma.user.create({ data: { email: req.body.email } });
    return reply.code(201).send(user);
  });
}

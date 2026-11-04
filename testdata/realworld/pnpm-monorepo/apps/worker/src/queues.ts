import { Queue } from "bullmq";
import IORedis from "ioredis";

export const connection = new IORedis(process.env.REDIS_URL ?? "redis://localhost:6379", {
  maxRetriesPerRequest: null,
});

export const prefix = process.env.QUEUE_PREFIX ?? "acme";

export const emailQueue = new Queue("email", { connection, prefix });

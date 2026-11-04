import { Worker } from "bullmq";
import nodemailer from "nodemailer";
import { connection, prefix } from "./queues.js";

const concurrency = Number(process.env.WORKER_CONCURRENCY || 5);

const transport = nodemailer.createTransport(process.env.SMTP_URL);

const worker = new Worker(
  "email",
  async (job) => {
    const { to, subject, text } = job.data as { to: string; subject: string; text: string };
    await transport.sendMail({ from: "no-reply@example.com", to, subject, text });
  },
  { connection, prefix, concurrency },
);

worker.on("failed", (job, err) => {
  console.error(`job ${job?.id} failed: ${err.message}`);
});

async function shutdown() {
  await worker.close();
  await connection.quit();
  process.exit(0);
}

process.on("SIGTERM", shutdown);
process.on("SIGINT", shutdown);

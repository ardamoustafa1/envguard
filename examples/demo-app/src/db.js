const { Pool } = require("pg");

const logLevel = process.env.LOG_LEVEL ?? "info";
const debug = process.env.DEBUG === "true";
const sessionSecret = process.env.SESSION_SECRET;
const redisUrl = process.env.REDIS_URL;
const sentry = process.env.SENTRY_DSN;

async function connect(url) {
  const pool = new Pool({ connectionString: url });
  if (debug) console.log("db", logLevel, Boolean(sessionSecret), redisUrl, sentry);
  return pool.connect();
}

module.exports = { connect };

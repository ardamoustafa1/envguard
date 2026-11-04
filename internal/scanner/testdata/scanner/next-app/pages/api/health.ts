export default function handler() {
  return { db: process.env.REAL_DB_FROM_API };
}

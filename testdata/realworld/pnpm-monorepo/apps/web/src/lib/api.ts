import { publicEnv } from "./env";

export interface User {
  id: string;
  email: string;
}

export async function listUsers(): Promise<User[]> {
  const base = typeof window === "undefined" ? process.env.API_INTERNAL_URL : publicEnv.apiUrl;
  const res = await fetch(`${base}/users`, { cache: "no-store" });
  if (!res.ok) throw new Error(`API responded with ${res.status}`);
  return res.json();
}

import { loadEnv } from "@acme/config";

export const config = loadEnv(process.env);

export const isProduction = config.NODE_ENV === "production";

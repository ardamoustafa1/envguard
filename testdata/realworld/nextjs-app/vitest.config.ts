import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";
import path from "node:path";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { "@": path.resolve(__dirname, ".") },
  },
  test: {
    environment: "node",
    include: ["tests/unit/**/*.test.ts"],
    env: {
      SKIP_ENV_VALIDATION: "1",
    },
    coverage: {
      enabled: !!process.env.CI,
      reporter: ["text", "lcov"],
    },
  },
});

import type { NextConfig } from "next";

// Validate env at build time.
import "./lib/env";

const nextConfig: NextConfig = {
  output: "standalone",
  reactStrictMode: true,
  poweredByHeader: false,
  experimental: {
    typedRoutes: true,
  },
  images: {
    remotePatterns: [{ protocol: "https", hostname: "avatars.githubusercontent.com" }],
  },
  async headers() {
    return [
      {
        source: "/(.*)",
        headers: [
          { key: "X-Frame-Options", value: "DENY" },
          { key: "X-Content-Type-Options", value: "nosniff" },
        ],
      },
    ];
  },
};

export default process.env.ANALYZE === "true" ? withAnalyzer(nextConfig) : nextConfig;

function withAnalyzer(config: NextConfig): NextConfig {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const analyzer = require("@next/bundle-analyzer")({ enabled: true });
  return analyzer(config);
}

import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Enable standalone output for efficient Docker builds
  output: "standalone",

  // Note: Grafana proxy is handled by /app/grafana/[...path]/route.ts
  // This allows us to add auth headers when proxying to Grafana
};

export default nextConfig;

import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Note: We don't use standalone output because we have a custom server.js
  // that handles WebSocket proxying for agent connections.
  // The custom server wraps Next.js and intercepts WebSocket upgrades.

  // Note: Grafana proxy is handled by /app/grafana/[...path]/route.ts
  // This allows us to add auth headers when proxying to Grafana
};

export default nextConfig;

import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Enable standalone output for efficient Docker builds
  output: "standalone",
};

export default nextConfig;

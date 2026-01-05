import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Enable standalone output for efficient Docker builds
  output: "standalone",

  // Proxy /grafana/* to the Grafana service
  // This allows Grafana embeds to work when Grafana is configured with serve_from_sub_path
  async rewrites() {
    const grafanaUrl = process.env.NEXT_PUBLIC_GRAFANA_URL;
    if (!grafanaUrl) {
      return [];
    }
    return [
      {
        source: "/grafana/:path*",
        destination: `${grafanaUrl}/grafana/:path*`,
      },
    ];
  },
};

export default nextConfig;

/**
 * Runtime configuration endpoint.
 *
 * Returns configuration values that need to be read at runtime
 * rather than build time. This is necessary for Kubernetes deployments
 * where config is provided via ConfigMaps/environment variables.
 */

import { NextResponse } from "next/server";

export async function GET() {
  return NextResponse.json({
    demoMode: process.env.NEXT_PUBLIC_DEMO_MODE === "true",
    readOnlyMode: process.env.NEXT_PUBLIC_READ_ONLY_MODE === "true",
    readOnlyMessage: process.env.NEXT_PUBLIC_READ_ONLY_MESSAGE || "This dashboard is in read-only mode",
    // WebSocket proxy URL for agent console connections (runtime config for K8s deployments)
    wsProxyUrl: process.env.NEXT_PUBLIC_WS_PROXY_URL || "",
    // Grafana URL for metrics dashboards (runtime config for K8s/Tilt deployments)
    grafanaUrl: process.env.NEXT_PUBLIC_GRAFANA_URL || "",
  });
}

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
    devMode: process.env.NEXT_PUBLIC_DEV_MODE === "true",
    demoMode: process.env.NEXT_PUBLIC_DEMO_MODE === "true",
    readOnlyMode: process.env.NEXT_PUBLIC_READ_ONLY_MODE === "true",
    readOnlyMessage: process.env.NEXT_PUBLIC_READ_ONLY_MESSAGE || "This dashboard is in read-only mode",
    // WebSocket proxy URL for agent console connections (runtime config for K8s deployments)
    wsProxyUrl: process.env.NEXT_PUBLIC_WS_PROXY_URL || "",
    // Grafana URL for metrics dashboards (runtime config for K8s/Tilt deployments)
    grafanaUrl: process.env.NEXT_PUBLIC_GRAFANA_URL || "",
    // Loki/Tempo enabled flags (for showing links to log/trace explorers)
    lokiEnabled: process.env.NEXT_PUBLIC_LOKI_ENABLED === "true",
    tempoEnabled: process.env.NEXT_PUBLIC_TEMPO_ENABLED === "true",
    // Enterprise features configuration
    // enterpriseEnabled: true if enterprise CRDs and controllers are deployed (enterprise.enabled=true in Helm)
    // hideEnterprise: true to completely hide enterprise features instead of showing upgrade prompts
    enterpriseEnabled: process.env.NEXT_PUBLIC_ENTERPRISE_ENABLED === "true",
    hideEnterprise: process.env.NEXT_PUBLIC_HIDE_ENTERPRISE === "true",
  });
}

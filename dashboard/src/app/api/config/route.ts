/**
 * Runtime configuration endpoint.
 *
 * Returns configuration values that need to be read at runtime
 * rather than build time. This is necessary for Kubernetes deployments
 * where config is provided via ConfigMaps/environment variables.
 */

import { NextResponse } from "next/server";
import { getEffectiveLicense } from "@/lib/license/resolve-server";
import { resolveBrand, applyEntitlement } from "@/lib/branding/resolve-server";

const MIN_POLL_INTERVAL_SECONDS = 15;
const DEFAULT_POLL_INTERVAL_SECONDS = 60;

function parseSessionPollInterval(): number {
  const raw = process.env.OMNIA_SESSION_POLL_INTERVAL_SECONDS;
  if (!raw) return DEFAULT_POLL_INTERVAL_SECONDS;
  const parsed = Number.parseInt(raw, 10);
  if (Number.isNaN(parsed)) return DEFAULT_POLL_INTERVAL_SECONDS;
  return Math.max(MIN_POLL_INTERVAL_SECONDS, parsed);
}

export async function GET() {
  const license = await getEffectiveLicense();
  const demoMode = process.env.NEXT_PUBLIC_DEMO_MODE === "true";
  // Presets (NEXT_PUBLIC_BRAND_PRESET) are a dev/demo shortcut; a real
  // deployment still resolves from the full NEXT_PUBLIC_BRAND_* env set.
  const brand = applyEntitlement(
    resolveBrand(process.env, { allowPreset: demoMode }),
    license,
  );
  return NextResponse.json({
    devMode: process.env.NEXT_PUBLIC_DEV_MODE === "true",
    demoMode,
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
    // Session expiry detection
    authMode: process.env.OMNIA_AUTH_MODE || "anonymous",
    sessionPollIntervalSeconds: parseSessionPollInterval(),
    // White-label branding (license-gated server-side; Omnia default otherwise)
    brand,
  });
}

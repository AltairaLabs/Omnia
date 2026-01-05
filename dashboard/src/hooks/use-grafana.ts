/**
 * Hook for Grafana integration configuration.
 *
 * When NEXT_PUBLIC_GRAFANA_URL is set, the dashboard can embed Grafana panels
 * for metrics visualization. When not set, fallback UI is shown.
 *
 * Environment variables:
 * - NEXT_PUBLIC_GRAFANA_URL: Base URL of Grafana instance (e.g., http://grafana:3000)
 * - NEXT_PUBLIC_GRAFANA_ORG_ID: Grafana organization ID (default: 1)
 */

export interface GrafanaConfig {
  /** Whether Grafana integration is enabled */
  enabled: boolean;
  /** Base URL of the Grafana instance */
  baseUrl: string | null;
  /** Organization ID for multi-org setups */
  orgId: number;
}

export interface GrafanaPanelOptions {
  /** Dashboard UID */
  dashboardUid: string;
  /** Panel ID within the dashboard */
  panelId: number;
  /** Time range (e.g., "now-1h", "now-24h") */
  from?: string;
  to?: string;
  /** Refresh interval (e.g., "5s", "1m") */
  refresh?: string;
  /** Theme: light or dark */
  theme?: "light" | "dark";
  /** Template variables */
  vars?: Record<string, string>;
}

// Pre-defined dashboard UIDs (these should match provisioned dashboards)
export const GRAFANA_DASHBOARDS = {
  AGENT_OVERVIEW: "omnia-agent-overview",
  TOKEN_USAGE: "omnia-token-usage",
  SESSION_METRICS: "omnia-sessions",
  SYSTEM_OVERVIEW: "omnia-system",
} as const;

// Pre-defined panel IDs within dashboards
export const GRAFANA_PANELS = {
  // Agent Overview dashboard
  REQUESTS_PER_SECOND: 1,
  LATENCY_HISTOGRAM: 2,
  ERROR_RATE: 3,
  ACTIVE_CONNECTIONS: 4,
  // Token Usage dashboard
  TOKEN_USAGE_OVER_TIME: 1,
  INPUT_VS_OUTPUT: 2,
  COST_OVER_TIME: 3,
  CACHE_HIT_RATE: 4,
  // Session Metrics dashboard
  ACTIVE_SESSIONS: 1,
  SESSION_DURATION: 2,
  MESSAGES_PER_SESSION: 3,
  // System Overview dashboard
  TOTAL_REQUESTS: 1,
  TOTAL_AGENTS: 2,
  SYSTEM_LATENCY: 3,
  SYSTEM_ERRORS: 4,
} as const;

/**
 * Returns Grafana configuration from environment variables.
 */
export function useGrafana(): GrafanaConfig {
  const baseUrl = process.env.NEXT_PUBLIC_GRAFANA_URL || null;
  const orgId = parseInt(process.env.NEXT_PUBLIC_GRAFANA_ORG_ID || "1", 10);

  return {
    enabled: !!baseUrl,
    baseUrl,
    orgId,
  };
}

/**
 * Builds a Grafana panel embed URL.
 *
 * @param config - Grafana configuration
 * @param options - Panel options
 * @returns The embed URL or null if Grafana is not enabled
 */
export function buildPanelUrl(
  config: GrafanaConfig,
  options: GrafanaPanelOptions
): string | null {
  if (!config.enabled || !config.baseUrl) {
    return null;
  }

  const {
    dashboardUid,
    panelId,
    from = "now-1h",
    to = "now",
    refresh = "30s",
    theme = "dark",
    vars = {},
  } = options;

  // Build the solo panel URL
  const url = new URL(
    `/d-solo/${dashboardUid}`,
    config.baseUrl
  );

  // Add query parameters
  url.searchParams.set("orgId", config.orgId.toString());
  url.searchParams.set("panelId", panelId.toString());
  url.searchParams.set("from", from);
  url.searchParams.set("to", to);
  url.searchParams.set("refresh", refresh);
  url.searchParams.set("theme", theme);

  // Add template variables
  for (const [key, value] of Object.entries(vars)) {
    url.searchParams.set(`var-${key}`, value);
  }

  return url.toString();
}

/**
 * Builds a full Grafana dashboard URL.
 *
 * @param config - Grafana configuration
 * @param dashboardUid - Dashboard UID
 * @param vars - Template variables
 * @returns The dashboard URL or null if Grafana is not enabled
 */
export function buildDashboardUrl(
  config: GrafanaConfig,
  dashboardUid: string,
  vars: Record<string, string> = {}
): string | null {
  if (!config.enabled || !config.baseUrl) {
    return null;
  }

  const url = new URL(`/d/${dashboardUid}`, config.baseUrl);
  url.searchParams.set("orgId", config.orgId.toString());

  for (const [key, value] of Object.entries(vars)) {
    url.searchParams.set(`var-${key}`, value);
  }

  return url.toString();
}

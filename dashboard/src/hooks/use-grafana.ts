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

// Pre-defined dashboard UIDs (must match provisioned dashboards in Helm chart)
export const GRAFANA_DASHBOARDS = {
  /** System-wide overview: requests, latency, costs, tokens */
  OVERVIEW: "omnia-overview",
  /** Cost analysis: by model, agent, trends */
  COSTS: "omnia-costs",
  /** Per-agent detail with template variables */
  AGENT_DETAIL: "omnia-agent-detail",
  /** Logs explorer (Loki) */
  LOGS: "omnia-logs",
} as const;

// Panel IDs within the Overview dashboard
export const OVERVIEW_PANELS = {
  REQUESTS_PER_SEC: 1,
  P95_LATENCY: 2,
  COST_24H: 3,
  TOKENS_PER_MIN: 4,
  REQUEST_RATE_BY_AGENT: 5,
  GENERATION_LATENCY: 6,
  TOKEN_USAGE_BY_AGENT: 7,
  TOOL_CALLS_BY_AGENT: 8,
} as const;

// Panel IDs within the Agent Detail dashboard
export const AGENT_DETAIL_PANELS = {
  REQUESTS_PER_SEC: 1,
  P95_LATENCY: 2,
  ERROR_RATE: 3,
  ACTIVE_CONNECTIONS: 4,
  REQUEST_RATE: 5,
  LATENCY_DISTRIBUTION: 6,
  TOKEN_USAGE: 7,
  TOOL_CALLS: 8,
  RECENT_LOGS: 9,
  RECENT_TRACES: 10,
} as const;

// Panel IDs within the Costs dashboard
export const COSTS_PANELS = {
  COST_BY_MODEL: 1,
  COST_BY_AGENT: 2,
  TOTAL_COST_7D: 3,
  HOURLY_COST_TREND: 4,
  INPUT_TOKENS_BY_MODEL: 5,
  OUTPUT_TOKENS_BY_MODEL: 6,
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
 * Uses /grafana/ subpath since Grafana is configured with serve_from_sub_path.
 * The Next.js rewrite proxies /grafana/* to the actual Grafana service.
 *
 * @param config - Grafana configuration
 * @param options - Panel options
 * @returns The embed URL or null if Grafana is not enabled
 */
export function buildPanelUrl(
  config: GrafanaConfig,
  options: GrafanaPanelOptions
): string | null {
  if (!config.enabled) {
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

  // Build query params
  const params = new URLSearchParams();
  params.set("orgId", config.orgId.toString());
  params.set("panelId", panelId.toString());
  params.set("from", from);
  params.set("to", to);
  params.set("refresh", refresh);
  params.set("theme", theme);

  // Add template variables
  for (const [key, value] of Object.entries(vars)) {
    params.set(`var-${key}`, value);
  }

  // Use relative /grafana/ path - Next.js rewrites this to the actual Grafana service
  return `/grafana/d-solo/${dashboardUid}?${params.toString()}`;
}

/**
 * Builds a full Grafana dashboard URL.
 *
 * Uses /grafana/ subpath since Grafana is configured with serve_from_sub_path.
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
  if (!config.enabled) {
    return null;
  }

  const params = new URLSearchParams();
  params.set("orgId", config.orgId.toString());

  for (const [key, value] of Object.entries(vars)) {
    params.set(`var-${key}`, value);
  }

  // Use relative /grafana/ path with slug (use uid as slug for provisioned dashboards)
  return `/grafana/d/${dashboardUid}/_?${params.toString()}`;
}

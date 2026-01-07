/**
 * Hook for Grafana integration configuration.
 *
 * When NEXT_PUBLIC_GRAFANA_URL is set, the dashboard can embed Grafana panels
 * for metrics visualization. When not set, fallback UI is shown.
 *
 * Environment variables:
 * - NEXT_PUBLIC_GRAFANA_URL: Base URL of Grafana instance (e.g., http://grafana:3000)
 * - NEXT_PUBLIC_GRAFANA_PATH: Subpath on the remote server (default: /grafana/)
 * - NEXT_PUBLIC_GRAFANA_ORG_ID: Grafana organization ID (default: 1)
 */

export interface GrafanaConfig {
  /** Whether Grafana integration is enabled */
  enabled: boolean;
  /** Base URL of the Grafana instance */
  baseUrl: string | null;
  /** Subpath where Grafana is served on the remote server (e.g., /grafana/, /monitoring/) */
  remotePath: string;
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
 * Normalize a path to ensure it starts and ends with /.
 */
function normalizePath(path: string): string {
  let normalized = path.trim();
  if (!normalized.startsWith("/")) {
    normalized = "/" + normalized;
  }
  if (!normalized.endsWith("/")) {
    normalized = normalized + "/";
  }
  return normalized;
}

/**
 * Returns Grafana configuration from environment variables.
 */
export function useGrafana(): GrafanaConfig {
  const baseUrl = process.env.NEXT_PUBLIC_GRAFANA_URL || null;
  const remotePath = normalizePath(
    process.env.NEXT_PUBLIC_GRAFANA_PATH || "/grafana/"
  );
  const orgId = parseInt(process.env.NEXT_PUBLIC_GRAFANA_ORG_ID || "1", 10);

  return {
    enabled: !!baseUrl,
    baseUrl,
    remotePath,
    orgId,
  };
}

/**
 * Builds a Grafana panel embed URL.
 *
 * Uses the configured NEXT_PUBLIC_GRAFANA_URL directly for iframe embedding.
 * No proxy needed when Grafana has anonymous auth enabled.
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

  // Build absolute URL directly to Grafana (no proxy)
  const base = config.baseUrl.endsWith("/") ? config.baseUrl.slice(0, -1) : config.baseUrl;
  const path = config.remotePath.endsWith("/") ? config.remotePath.slice(0, -1) : config.remotePath;
  return `${base}${path}/d-solo/${dashboardUid}?${params.toString()}`;
}

/**
 * Builds a full Grafana dashboard URL.
 *
 * Uses the configured NEXT_PUBLIC_GRAFANA_URL directly.
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

  const params = new URLSearchParams();
  params.set("orgId", config.orgId.toString());

  for (const [key, value] of Object.entries(vars)) {
    params.set(`var-${key}`, value);
  }

  // Build absolute URL directly to Grafana (no proxy)
  const base = config.baseUrl.endsWith("/") ? config.baseUrl.slice(0, -1) : config.baseUrl;
  const path = config.remotePath.endsWith("/") ? config.remotePath.slice(0, -1) : config.remotePath;
  return `${base}${path}/d/${dashboardUid}/_?${params.toString()}`;
}

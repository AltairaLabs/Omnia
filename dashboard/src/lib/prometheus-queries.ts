/**
 * Centralized Prometheus query builder for Omnia metrics.
 *
 * All metric names and query patterns are defined here to ensure consistency
 * across the dashboard and make it easy to update if metric names change.
 */

// =============================================================================
// METRIC NAMES
// =============================================================================
// These are the metric names emitted by the Omnia components.
// If metric names change in the Go code, update them here.

/**
 * Agent metrics - emitted by the facade container.
 * Prefix: omnia_agent_*
 */
export const AGENT_METRICS = {
  /** Current number of active WebSocket connections (gauge) */
  CONNECTIONS_ACTIVE: "omnia_agent_connections_active",
  /** Total number of WebSocket connections since startup (counter) */
  CONNECTIONS_TOTAL: "omnia_agent_connections_total",
  /** Total number of WebSocket messages received (counter) */
  MESSAGES_RECEIVED: "omnia_agent_messages_received_total",
  /** Total number of WebSocket messages sent (counter) */
  MESSAGES_SENT: "omnia_agent_messages_sent_total",
  /** Request processing duration histogram (histogram) */
  REQUEST_DURATION: "omnia_agent_request_duration_seconds",
  /** Current number of requests being processed (gauge) */
  REQUESTS_INFLIGHT: "omnia_agent_requests_inflight",
  /** Total number of requests processed (counter) */
  REQUESTS_TOTAL: "omnia_agent_requests_total",
  /** Current number of active sessions (gauge) */
  SESSIONS_ACTIVE: "omnia_agent_sessions_active",
} as const;

/**
 * Facade-specific metrics - file transfer operations.
 * Prefix: omnia_facade_*
 */
export const FACADE_METRICS = {
  /** Total bytes downloaded (counter) */
  DOWNLOAD_BYTES: "omnia_facade_download_bytes_total",
  /** Total bytes sent as media chunks (counter) */
  MEDIA_CHUNK_BYTES: "omnia_facade_media_chunk_bytes_total",
  /** Total bytes uploaded (counter) */
  UPLOAD_BYTES: "omnia_facade_upload_bytes_total",
  /** Upload duration histogram (histogram) */
  UPLOAD_DURATION: "omnia_facade_upload_duration_seconds",
} as const;

/**
 * LLM metrics - emitted by the runtime container.
 * Prefix: omnia_llm_*
 */
export const LLM_METRICS = {
  /** Total cache hits (counter) */
  CACHE_HITS: "omnia_llm_cache_hits_total",
  /** Total cost in USD (counter) */
  COST_USD: "omnia_llm_cost_usd_total",
  /** Total input tokens (counter) */
  INPUT_TOKENS: "omnia_llm_input_tokens_total",
  /** Total output tokens (counter) */
  OUTPUT_TOKENS: "omnia_llm_output_tokens_total",
  /** LLM request duration histogram (histogram) */
  REQUEST_DURATION: "omnia_llm_request_duration_seconds",
  /** Total LLM requests (counter) */
  REQUESTS_TOTAL: "omnia_llm_requests_total",
} as const;

// =============================================================================
// LABEL NAMES
// =============================================================================

export const LABELS = {
  AGENT: "agent",
  NAMESPACE: "namespace",
  PROVIDER: "provider",
  MODEL: "model",
  STATUS: "status",
  HANDLER: "handler",
  // CRD reference labels (const labels set per-deployment, useful for Grafana queries)
  PROMPTPACK_NAME: "promptpack_name",
  PROMPTPACK_NAMESPACE: "promptpack_namespace",
  PROVIDER_REF_NAME: "provider_ref_name",
  PROVIDER_REF_NAMESPACE: "provider_ref_namespace",
} as const;

// =============================================================================
// QUERY BUILDER
// =============================================================================

export interface QueryFilter {
  agent?: string;
  namespace?: string;
  provider?: string;
  model?: string;
  status?: string;
}

/**
 * Build a label filter string for PromQL queries.
 * @example buildFilter({ agent: "my-agent", namespace: "prod" }) => 'agent="my-agent",namespace="prod"'
 */
export function buildFilter(filter: QueryFilter): string {
  const parts: string[] = [];
  if (filter.agent) parts.push(`${LABELS.AGENT}="${filter.agent}"`);
  if (filter.namespace) parts.push(`${LABELS.NAMESPACE}="${filter.namespace}"`);
  if (filter.provider) parts.push(`${LABELS.PROVIDER}="${filter.provider}"`);
  if (filter.model) parts.push(`${LABELS.MODEL}="${filter.model}"`);
  if (filter.status) parts.push(`${LABELS.STATUS}="${filter.status}"`);
  return parts.join(",");
}

/**
 * Wrap a metric name with optional filter.
 */
function metric(name: string, filter?: QueryFilter): string {
  const filterStr = filter ? buildFilter(filter) : "";
  return filterStr ? `${name}{${filterStr}}` : name;
}

// =============================================================================
// AGENT QUERIES
// =============================================================================

export const AgentQueries = {
  /**
   * Current active connections for an agent.
   */
  connectionsActive(filter?: QueryFilter): string {
    return `sum(${metric(AGENT_METRICS.CONNECTIONS_ACTIVE, filter)})`;
  },

  /**
   * Request rate (requests per second) over a time window.
   */
  requestRate(filter?: QueryFilter, window = "5m"): string {
    return `sum(rate(${metric(AGENT_METRICS.REQUESTS_TOTAL, filter)}[${window}]))`;
  },

  /**
   * P95 request latency in milliseconds.
   */
  p95Latency(filter?: QueryFilter, window = "5m"): string {
    const bucketMetric = `${AGENT_METRICS.REQUEST_DURATION}_bucket`;
    return `histogram_quantile(0.95, sum(rate(${metric(bucketMetric, filter)}[${window}])) by (le)) * 1000`;
  },

  /**
   * P99 request latency in milliseconds.
   */
  p99Latency(filter?: QueryFilter, window = "5m"): string {
    const bucketMetric = `${AGENT_METRICS.REQUEST_DURATION}_bucket`;
    return `histogram_quantile(0.99, sum(rate(${metric(bucketMetric, filter)}[${window}])) by (le)) * 1000`;
  },

  /**
   * Average request latency in milliseconds.
   */
  avgLatency(filter?: QueryFilter, window = "5m"): string {
    const sumMetric = `${AGENT_METRICS.REQUEST_DURATION}_sum`;
    const countMetric = `${AGENT_METRICS.REQUEST_DURATION}_count`;
    return `(sum(rate(${metric(sumMetric, filter)}[${window}])) / sum(rate(${metric(countMetric, filter)}[${window}]))) * 1000`;
  },

  /**
   * Current inflight requests.
   */
  inflightRequests(filter?: QueryFilter): string {
    return `sum(${metric(AGENT_METRICS.REQUESTS_INFLIGHT, filter)})`;
  },

  /**
   * Active sessions count.
   */
  activeSessions(filter?: QueryFilter): string {
    return `sum(${metric(AGENT_METRICS.SESSIONS_ACTIVE, filter)})`;
  },

  /**
   * Total requests (for display, not rate).
   */
  totalRequests(filter?: QueryFilter): string {
    return `sum(${metric(AGENT_METRICS.REQUESTS_TOTAL, filter)})`;
  },
};

// =============================================================================
// LLM QUERIES
// =============================================================================

export const LLMQueries = {
  /**
   * LLM request rate (requests per second).
   */
  requestRate(filter?: QueryFilter, window = "5m"): string {
    return `sum(rate(${metric(LLM_METRICS.REQUESTS_TOTAL, filter)}[${window}]))`;
  },

  /**
   * Error rate as a ratio (0-1).
   */
  errorRate(filter?: QueryFilter, window = "5m"): string {
    const errorFilter = { ...filter, status: "error" };
    return `sum(rate(${metric(LLM_METRICS.REQUESTS_TOTAL, errorFilter)}[${window}])) / sum(rate(${metric(LLM_METRICS.REQUESTS_TOTAL, filter)}[${window}]))`;
  },

  /**
   * Total input tokens.
   */
  inputTokens(filter?: QueryFilter): string {
    return `sum(${metric(LLM_METRICS.INPUT_TOKENS, filter)})`;
  },

  /**
   * Total output tokens.
   */
  outputTokens(filter?: QueryFilter): string {
    return `sum(${metric(LLM_METRICS.OUTPUT_TOKENS, filter)})`;
  },

  /**
   * Input token rate (tokens per second).
   */
  inputTokenRate(filter?: QueryFilter, window = "5m"): string {
    return `sum(rate(${metric(LLM_METRICS.INPUT_TOKENS, filter)}[${window}]))`;
  },

  /**
   * Output token rate (tokens per second).
   */
  outputTokenRate(filter?: QueryFilter, window = "5m"): string {
    return `sum(rate(${metric(LLM_METRICS.OUTPUT_TOKENS, filter)}[${window}]))`;
  },

  /**
   * Token increase over a time window (for charts).
   */
  inputTokenIncrease(filter?: QueryFilter, window = "5m"): string {
    return `sum(increase(${metric(LLM_METRICS.INPUT_TOKENS, filter)}[${window}]))`;
  },

  /**
   * Token increase over a time window (for charts).
   */
  outputTokenIncrease(filter?: QueryFilter, window = "5m"): string {
    return `sum(increase(${metric(LLM_METRICS.OUTPUT_TOKENS, filter)}[${window}]))`;
  },

  /**
   * Total cost in USD.
   */
  totalCost(filter?: QueryFilter): string {
    return `sum(${metric(LLM_METRICS.COST_USD, filter)})`;
  },

  /**
   * Cost increase over a time window.
   */
  costIncrease(filter?: QueryFilter, window = "24h"): string {
    return `sum(increase(${metric(LLM_METRICS.COST_USD, filter)}[${window}]))`;
  },

  /**
   * Cache hit count.
   */
  cacheHits(filter?: QueryFilter): string {
    return `sum(${metric(LLM_METRICS.CACHE_HITS, filter)})`;
  },

  /**
   * P95 LLM request latency in milliseconds.
   */
  p95Latency(filter?: QueryFilter, window = "5m"): string {
    const bucketMetric = `${LLM_METRICS.REQUEST_DURATION}_bucket`;
    return `histogram_quantile(0.95, sum(rate(${metric(bucketMetric, filter)}[${window}])) by (le)) * 1000`;
  },

  /**
   * Breakdown by provider/model for aggregation.
   */
  byProviderModel(metricName: string, filter?: QueryFilter): string {
    return `sum by (${LABELS.AGENT}, ${LABELS.NAMESPACE}, ${LABELS.PROVIDER}, ${LABELS.MODEL}) (${metric(metricName, filter)})`;
  },
};

// =============================================================================
// SYSTEM-WIDE QUERIES
// =============================================================================

export const SystemQueries = {
  /**
   * Total active connections across all agents.
   */
  totalConnections(): string {
    return AgentQueries.connectionsActive();
  },

  /**
   * Total request rate across all agents.
   */
  totalRequestRate(window = "5m"): string {
    return LLMQueries.requestRate(undefined, window);
  },

  /**
   * System-wide P95 latency.
   */
  systemP95Latency(window = "5m"): string {
    return AgentQueries.p95Latency(undefined, window);
  },

  /**
   * Total cost in last 24 hours.
   */
  cost24h(): string {
    return LLMQueries.costIncrease(undefined, "24h");
  },

  /**
   * Tokens per minute (combined input + output).
   * Uses increase() over 5m window for more reliable results with sparse data.
   */
  tokensPerMinute(): string {
    return `sum(increase(${LLM_METRICS.INPUT_TOKENS}[5m]) + increase(${LLM_METRICS.OUTPUT_TOKENS}[5m])) / 5`;
  },

  /**
   * Cost time series by provider (for charts).
   */
  costByProvider(window = "1h"): string {
    return `sum by (${LABELS.PROVIDER}) (increase(${LLM_METRICS.COST_USD}[${window}]))`;
  },
};

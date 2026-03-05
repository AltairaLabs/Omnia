/**
 * Prometheus client utilities for querying metrics via the server-side proxy.
 */

// Prometheus response types
export interface PrometheusResponse<T = PrometheusResult> {
  status: "success" | "error";
  data?: {
    resultType: "vector" | "matrix" | "scalar" | "string";
    result: T[];
  };
  errorType?: string;
  error?: string;
}

export interface PrometheusResult {
  metric: Record<string, string>;
  value?: [number, string]; // [timestamp, value] for instant queries
  values?: [number, string][]; // [[timestamp, value], ...] for range queries
}

export interface PrometheusVectorResult extends PrometheusResult {
  value: [number, string];
}

export interface PrometheusMatrixResult extends PrometheusResult {
  values: [number, string][];
}

/**
 * Timestamp type for Prometheus queries.
 * Can be a Date object, Unix timestamp (seconds), or RFC3339 string.
 */
export type PrometheusTimestamp = Date | number | string;

/**
 * Execute an instant Prometheus query.
 *
 * @param query - PromQL query string
 * @param time - Optional evaluation timestamp (Unix seconds or RFC3339)
 * @returns Prometheus query result
 */
export async function queryPrometheus(
  query: string,
  time?: string | number
): Promise<PrometheusResponse<PrometheusVectorResult>> {
  const params = new URLSearchParams({ query });
  if (time !== undefined) {
    params.set("time", String(time));
  }

  const response = await fetch(`/api/prometheus/query?${params}`);
  return response.json();
}

/**
 * Execute a range Prometheus query.
 *
 * @param query - PromQL query string
 * @param start - Start timestamp (Unix seconds, RFC3339, or Date)
 * @param end - End timestamp (Unix seconds, RFC3339, or Date)
 * @param step - Query resolution step (e.g., "1h", "15m", "60s")
 * @returns Prometheus range query result
 */
export async function queryPrometheusRange(
  query: string,
  start: PrometheusTimestamp,
  end: PrometheusTimestamp,
  step: string = "1h"
): Promise<PrometheusResponse<PrometheusMatrixResult>> {
  const formatTime = (t: PrometheusTimestamp): string => {
    if (t instanceof Date) {
      return (t.getTime() / 1000).toFixed(3);
    }
    return String(t);
  };

  const params = new URLSearchParams({
    query,
    start: formatTime(start),
    end: formatTime(end),
    step,
  });

  const response = await fetch(`/api/prometheus/query_range?${params}`);
  return response.json();
}

/** Prometheus metric type as reported by the metadata endpoint. */
export type PrometheusMetricType = "gauge" | "counter" | "histogram" | "summary" | "unknown";

interface PrometheusMetadataResponse {
  status: "success" | "error";
  data?: Record<string, Array<{ type: string; help: string; unit: string }>>;
  errorType?: string;
  error?: string;
}

/**
 * Query Prometheus metadata to get metric types.
 *
 * Returns a map of metric name to its Prometheus type.
 * If a metric filter is provided, only that metric's metadata is returned.
 */
export async function queryPrometheusMetadata(
  metric?: string,
): Promise<Record<string, PrometheusMetricType>> {
  const params = new URLSearchParams();
  if (metric) {
    params.set("metric", metric);
  }

  const url = params.toString()
    ? `/api/prometheus/metadata?${params}`
    : "/api/prometheus/metadata";
  const response = await fetch(url);
  const body: PrometheusMetadataResponse = await response.json();

  if (body.status !== "success" || !body.data) {
    return {};
  }

  const result: Record<string, PrometheusMetricType> = {};
  for (const [name, entries] of Object.entries(body.data)) {
    const rawType = entries[0]?.type?.toLowerCase() ?? "unknown";
    result[name] = isKnownMetricType(rawType) ? rawType : "unknown";
  }
  return result;
}

function isKnownMetricType(t: string): t is PrometheusMetricType {
  return t === "gauge" || t === "counter" || t === "histogram" || t === "summary";
}

/**
 * Check if Prometheus is available by making a simple query.
 *
 * @returns true if Prometheus is reachable
 */
export async function isPrometheusAvailable(): Promise<boolean> {
  try {
    const response = await queryPrometheus("up");
    return response.status === "success";
  } catch {
    return false;
  }
}

/**
 * Convert a Prometheus matrix result to time series data.
 *
 * @param result - Prometheus range query response
 * @returns Array of time series points
 */
export function matrixToTimeSeries(
  result: PrometheusResponse<PrometheusMatrixResult>
): Array<{ timestamp: Date; values: Record<string, number> }> {
  if (result.status !== "success" || !result.data?.result) {
    return [];
  }

  // Collect all timestamps and values
  const timeMap = new Map<number, Record<string, number>>();

  for (const series of result.data.result) {
    const seriesKey =
      series.metric.agent ||
      series.metric.provider ||
      series.metric.model ||
      "value";

    for (const [timestamp, value] of series.values || []) {
      if (!timeMap.has(timestamp)) {
        timeMap.set(timestamp, {});
      }
      timeMap.get(timestamp)![seriesKey] = Number.parseFloat(value) || 0;
    }
  }

  // Convert to array sorted by timestamp
  return Array.from(timeMap.entries())
    .sort(([a], [b]) => a - b)
    .map(([timestamp, values]) => ({
      timestamp: new Date(timestamp * 1000),
      values,
    }));
}

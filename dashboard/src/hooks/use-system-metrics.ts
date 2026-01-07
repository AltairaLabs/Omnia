/**
 * Hook for system-wide metrics from Prometheus.
 *
 * Fetches real-time metrics for the dashboard overview:
 * - Request rate across all agents
 * - P95 latency
 * - Cost in the last 24 hours
 * - Token throughput
 */

import { useQuery } from "@tanstack/react-query";
import {
  queryPrometheus,
  queryPrometheusRange,
  extractScalar,
  isPrometheusAvailable,
  type PrometheusVectorResult,
  type PrometheusMatrixResult,
} from "@/lib/prometheus";

export interface MetricDataPoint {
  time: string;
  value: number;
}

export interface SystemMetric {
  /** Current/latest value */
  current: number;
  /** Formatted display value */
  display: string;
  /** Time series data for sparkline (last hour, 5-minute intervals) */
  series: MetricDataPoint[];
  /** Unit for display */
  unit: string;
}

export interface SystemMetrics {
  available: boolean;
  requestsPerSec: SystemMetric;
  p95Latency: SystemMetric;
  cost24h: SystemMetric;
  tokensPerMin: SystemMetric;
}

const EMPTY_METRIC: SystemMetric = {
  current: 0,
  display: "--",
  series: [],
  unit: "",
};

const EMPTY_METRICS: SystemMetrics = {
  available: false,
  requestsPerSec: EMPTY_METRIC,
  p95Latency: EMPTY_METRIC,
  cost24h: EMPTY_METRIC,
  tokensPerMin: EMPTY_METRIC,
};

/**
 * Format a number for display with appropriate units.
 */
function formatValue(value: number, unit: string): string {
  if (unit === "req/s") {
    return value < 1 ? value.toFixed(2) : value.toFixed(1);
  }
  if (unit === "ms") {
    return value.toFixed(0);
  }
  if (unit === "$") {
    return `$${value.toFixed(2)}`;
  }
  if (unit === "tok/min") {
    if (value >= 1000000) return `${(value / 1000000).toFixed(1)}M`;
    if (value >= 1000) return `${(value / 1000).toFixed(1)}K`;
    return value.toFixed(0);
  }
  return value.toFixed(2);
}

/**
 * Convert Prometheus range result to MetricDataPoint array.
 */
function matrixToDataPoints(
  result: { status: string; data?: { result: PrometheusMatrixResult[] } },
  aggregateSum = true
): MetricDataPoint[] {
  if (result.status !== "success" || !result.data?.result?.length) {
    return [];
  }

  // Aggregate all series by timestamp
  const timeMap = new Map<number, number>();

  for (const series of result.data.result) {
    for (const [ts, val] of series.values || []) {
      const value = parseFloat(val) || 0;
      if (aggregateSum) {
        timeMap.set(ts, (timeMap.get(ts) || 0) + value);
      } else {
        // For latency, take max across series
        timeMap.set(ts, Math.max(timeMap.get(ts) || 0, value));
      }
    }
  }

  return Array.from(timeMap.entries())
    .sort(([a], [b]) => a - b)
    .map(([ts, value]) => ({
      time: new Date(ts * 1000).toLocaleTimeString([], {
        hour: "2-digit",
        minute: "2-digit",
      }),
      value,
    }));
}

/**
 * Fetch system metrics from Prometheus.
 */
async function fetchSystemMetrics(): Promise<SystemMetrics> {
  // Check availability first
  const available = await isPrometheusAvailable();
  if (!available) {
    return EMPTY_METRICS;
  }

  const now = new Date();
  const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000);
  const oneDayAgo = new Date(now.getTime() - 24 * 60 * 60 * 1000);

  try {
    // Fetch current values and time series in parallel
    const [
      reqRateCurrent,
      reqRateSeries,
      latencyCurrent,
      latencySeries,
      costTotal,
      costSeries,
      tokensCurrent,
      tokensSeries,
    ] = await Promise.all([
      // Request rate (requests per second)
      queryPrometheus("sum(rate(omnia_llm_requests_total[5m]))"),
      queryPrometheusRange(
        "sum(rate(omnia_llm_requests_total[5m]))",
        oneHourAgo,
        now,
        "5m"
      ),
      // P95 latency (milliseconds)
      queryPrometheus(
        "histogram_quantile(0.95, sum(rate(omnia_facade_request_duration_seconds_bucket[5m])) by (le)) * 1000"
      ),
      queryPrometheusRange(
        "histogram_quantile(0.95, sum(rate(omnia_facade_request_duration_seconds_bucket[5m])) by (le)) * 1000",
        oneHourAgo,
        now,
        "5m"
      ),
      // Cost 24h (sum of all costs)
      queryPrometheus("sum(increase(omnia_llm_cost_usd_total[24h]))"),
      queryPrometheusRange(
        "sum(increase(omnia_llm_cost_usd_total[1h]))",
        oneDayAgo,
        now,
        "1h"
      ),
      // Tokens per minute
      queryPrometheus(
        "sum(rate(omnia_llm_input_tokens_total[5m]) + rate(omnia_llm_output_tokens_total[5m])) * 60"
      ),
      queryPrometheusRange(
        "sum(rate(omnia_llm_input_tokens_total[5m]) + rate(omnia_llm_output_tokens_total[5m])) * 60",
        oneHourAgo,
        now,
        "5m"
      ),
    ]);

    // Extract current values
    const reqRate = extractScalarFromResult(reqRateCurrent);
    const latency = extractScalarFromResult(latencyCurrent);
    const cost = extractScalarFromResult(costTotal);
    const tokens = extractScalarFromResult(tokensCurrent);

    return {
      available: true,
      requestsPerSec: {
        current: reqRate,
        display: formatValue(reqRate, "req/s"),
        series: matrixToDataPoints(reqRateSeries),
        unit: "req/s",
      },
      p95Latency: {
        current: latency,
        display: formatValue(latency, "ms"),
        series: matrixToDataPoints(latencySeries, false),
        unit: "ms",
      },
      cost24h: {
        current: cost,
        display: formatValue(cost, "$"),
        series: matrixToDataPoints(costSeries),
        unit: "$",
      },
      tokensPerMin: {
        current: tokens,
        display: formatValue(tokens, "tok/min"),
        series: matrixToDataPoints(tokensSeries),
        unit: "tok/min",
      },
    };
  } catch (error) {
    console.error("Failed to fetch system metrics:", error);
    return EMPTY_METRICS;
  }
}

/**
 * Helper to extract scalar from instant query result.
 */
function extractScalarFromResult(result: {
  status: string;
  data?: { result: PrometheusVectorResult[] };
}): number {
  if (result.status !== "success" || !result.data?.result?.length) {
    return 0;
  }
  // Sum all results (in case there are multiple)
  return result.data.result.reduce((sum, item) => {
    const val = parseFloat(item.value?.[1] || "0");
    return sum + (isNaN(val) ? 0 : val);
  }, 0);
}

/**
 * Hook to fetch and cache system metrics.
 *
 * Refreshes every 30 seconds.
 */
export function useSystemMetrics() {
  return useQuery({
    queryKey: ["system-metrics"],
    queryFn: fetchSystemMetrics,
    refetchInterval: 30000, // Refresh every 30 seconds
    staleTime: 15000, // Consider stale after 15 seconds
  });
}

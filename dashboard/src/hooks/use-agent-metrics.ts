/**
 * Hook for per-agent metrics from Prometheus.
 *
 * Fetches metrics for a specific agent:
 * - Request rate
 * - P95 latency
 * - Error rate
 * - Active connections
 * - Token usage over time
 */

import { useQuery } from "@tanstack/react-query";
import {
  queryPrometheus,
  queryPrometheusRange,
  isPrometheusAvailable,
  type PrometheusVectorResult,
  type PrometheusMatrixResult,
} from "@/lib/prometheus";
import { AgentQueries, LLMQueries } from "@/lib/prometheus-queries";
import { useDemoMode } from "./use-runtime-config";

export interface MetricTimeSeriesPoint {
  time: string;
  value: number;
}

export interface TokenUsagePoint {
  time: string;
  input: number;
  output: number;
}

export interface AgentMetric {
  current: number;
  display: string;
  series: MetricTimeSeriesPoint[];
  unit: string;
}

export interface AgentMetrics {
  available: boolean;
  isDemo: boolean;
  requestsPerSec: AgentMetric;
  p95Latency: AgentMetric;
  errorRate: AgentMetric;
  activeConnections: AgentMetric;
  tokenUsage: TokenUsagePoint[];
}

const EMPTY_METRIC: AgentMetric = {
  current: 0,
  display: "--",
  series: [],
  unit: "",
};

const EMPTY_METRICS: AgentMetrics = {
  available: false,
  isDemo: false,
  requestsPerSec: EMPTY_METRIC,
  p95Latency: EMPTY_METRIC,
  errorRate: EMPTY_METRIC,
  activeConnections: EMPTY_METRIC,
  tokenUsage: [],
};

/**
 * Generate mock metrics for demo mode.
 */
function generateMockMetrics(agentName: string): AgentMetrics {
  const now = new Date();
  const series: MetricTimeSeriesPoint[] = [];
  const tokenUsage: TokenUsagePoint[] = [];

  // Generate 1 hour of 5-minute data points
  for (let i = 11; i >= 0; i--) {
    const time = new Date(now.getTime() - i * 5 * 60 * 1000);
    const timeStr = time.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });

    const seriesValue = Math.random() * 10 + 5; // NOSONAR - mock data (5-15 req/s)
    series.push({
      time: timeStr,
      value: seriesValue,
    });

    const inputTokens = Math.floor(Math.random() * 5000) + 2000; // NOSONAR - mock data
    const outputTokens = Math.floor(Math.random() * 2000) + 500; // NOSONAR - mock data
    tokenUsage.push({
      time: timeStr,
      input: inputTokens,
      output: outputTokens,
    });
  }

  // Seed randomness based on agent name for consistent demo data
  const seed = agentName.split("").reduce((a, b) => a + (b.codePointAt(0) ?? 0), 0);
  const baseReqRate = (seed % 10) + 5;
  const baseLatency = (seed % 50) + 100;
  const baseErrorRate = (seed % 5) / 100;
  const baseConnections = (seed % 20) + 5;

  const reqRateCurrent = baseReqRate + Math.random() * 2; // NOSONAR - mock data
  const reqRateDisplay = baseReqRate + Math.random() * 2; // NOSONAR - mock data
  const latencyCurrent = baseLatency + Math.random() * 20; // NOSONAR - mock data
  const latencyDisplay = baseLatency + Math.random() * 20; // NOSONAR - mock data

  return {
    available: true,
    isDemo: true,
    requestsPerSec: {
      current: reqRateCurrent,
      display: `${reqRateDisplay.toFixed(1)}`,
      series: series.map((s) => ({ ...s, value: baseReqRate + Math.random() * 5 })), // NOSONAR
      unit: "req/s",
    },
    p95Latency: {
      current: latencyCurrent,
      display: `${Math.round(latencyDisplay)}`,
      series: series.map((s) => ({ ...s, value: baseLatency + Math.random() * 30 })), // NOSONAR
      unit: "ms",
    },
    errorRate: {
      current: baseErrorRate,
      display: `${(baseErrorRate * 100).toFixed(2)}%`,
      series: series.map((s) => ({ ...s, value: baseErrorRate + Math.random() * 0.01 })), // NOSONAR
      unit: "%",
    },
    activeConnections: {
      current: baseConnections,
      display: `${baseConnections}`,
      series: series.map((s) => ({ ...s, value: baseConnections + Math.floor(Math.random() * 5) })), // NOSONAR
      unit: "",
    },
    tokenUsage,
  };
}

// Cache mock data per agent
const mockDataCache = new Map<string, AgentMetrics>();

function getMockMetrics(agentName: string, namespace: string): AgentMetrics {
  const key = `${namespace}/${agentName}`;
  if (!mockDataCache.has(key)) {
    mockDataCache.set(key, generateMockMetrics(agentName));
  }
  return mockDataCache.get(key)!;
}

/**
 * Format a number for display.
 */
function formatValue(value: number, unit: string): string {
  if (unit === "req/s") {
    return value < 1 ? value.toFixed(2) : value.toFixed(1);
  }
  if (unit === "ms") {
    return Math.round(value).toString();
  }
  if (unit === "%") {
    return `${(value * 100).toFixed(2)}%`;
  }
  return Math.round(value).toString();
}

/**
 * Convert Prometheus range result to time series points.
 */
function matrixToSeries(
  result: { status: string; data?: { result: PrometheusMatrixResult[] } }
): MetricTimeSeriesPoint[] {
  if (result.status !== "success" || !result.data?.result?.length) {
    return [];
  }

  const timeMap = new Map<number, number>();

  for (const series of result.data.result) {
    for (const [ts, val] of series.values || []) {
      const value = Number.parseFloat(val) || 0;
      timeMap.set(ts, (timeMap.get(ts) || 0) + value);
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

type MetricsPrometheusResult = { status: string; data?: { result: PrometheusMatrixResult[] } };
type TokenEntry = { input: number; output: number };

/**
 * Process token metrics from Prometheus result.
 */
function processTokenResult(
  result: MetricsPrometheusResult,
  timeMap: Map<number, TokenEntry>,
  field: "input" | "output"
): void {
  if (result.status !== "success" || !result.data?.result) return;

  for (const series of result.data.result) {
    for (const [ts, val] of series.values || []) {
      if (!timeMap.has(ts)) {
        timeMap.set(ts, { input: 0, output: 0 });
      }
      timeMap.get(ts)![field] += Number.parseFloat(val) || 0;
    }
  }
}

/**
 * Convert Prometheus range results to token usage points.
 */
function matrixToTokenUsage(
  inputResult: MetricsPrometheusResult,
  outputResult: MetricsPrometheusResult
): TokenUsagePoint[] {
  const timeMap = new Map<number, TokenEntry>();

  processTokenResult(inputResult, timeMap, "input");
  processTokenResult(outputResult, timeMap, "output");

  return Array.from(timeMap.entries())
    .sort(([a], [b]) => a - b)
    .map(([ts, data]) => ({
      time: new Date(ts * 1000).toLocaleTimeString([], {
        hour: "2-digit",
        minute: "2-digit",
      }),
      input: Math.round(data.input),
      output: Math.round(data.output),
    }));
}

/**
 * Extract scalar from instant query.
 */
function extractScalar(result: {
  status: string;
  data?: { result: PrometheusVectorResult[] };
}): number {
  if (result.status !== "success" || !result.data?.result?.length) {
    return 0;
  }
  return result.data.result.reduce((sum, item) => {
    const val = Number.parseFloat(item.value?.[1] || "0");
    return sum + (Number.isNaN(val) ? 0 : val);
  }, 0);
}

/**
 * Fetch agent metrics from Prometheus.
 */
async function fetchAgentMetrics(
  agentName: string,
  namespace: string
): Promise<AgentMetrics> {
  const available = await isPrometheusAvailable();
  if (!available) {
    return EMPTY_METRICS;
  }

  const now = new Date();
  const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000);
  const filter = { agent: agentName, namespace };

  try {
    const [
      // Current values
      reqRateCurrent,
      latencyCurrent,
      errorRateCurrent,
      connectionsCurrent,
      // Time series
      reqRateSeries,
      latencySeries,
      errorRateSeries,
      connectionsSeries,
      inputTokensSeries,
      outputTokensSeries,
    ] = await Promise.all([
      // Request rate (using LLM requests)
      queryPrometheus(LLMQueries.requestRate(filter)),
      // P95 latency (using agent request duration)
      queryPrometheus(AgentQueries.p95Latency(filter)),
      // Error rate
      queryPrometheus(LLMQueries.errorRate(filter)),
      // Active connections
      queryPrometheus(AgentQueries.connectionsActive(filter)),
      // Time series
      queryPrometheusRange(LLMQueries.requestRate(filter), oneHourAgo, now, "5m"),
      queryPrometheusRange(AgentQueries.p95Latency(filter), oneHourAgo, now, "5m"),
      queryPrometheusRange(LLMQueries.errorRate(filter), oneHourAgo, now, "5m"),
      queryPrometheusRange(AgentQueries.connectionsActive(filter), oneHourAgo, now, "5m"),
      queryPrometheusRange(LLMQueries.inputTokenIncrease(filter), oneHourAgo, now, "5m"),
      queryPrometheusRange(LLMQueries.outputTokenIncrease(filter), oneHourAgo, now, "5m"),
    ]);

    const reqRate = extractScalar(reqRateCurrent);
    const latency = extractScalar(latencyCurrent);
    const errorRate = extractScalar(errorRateCurrent);
    const connections = extractScalar(connectionsCurrent);

    return {
      available: true,
      isDemo: false,
      requestsPerSec: {
        current: reqRate,
        display: formatValue(reqRate, "req/s"),
        series: matrixToSeries(reqRateSeries),
        unit: "req/s",
      },
      p95Latency: {
        current: latency,
        display: formatValue(latency, "ms"),
        series: matrixToSeries(latencySeries),
        unit: "ms",
      },
      errorRate: {
        current: Number.isNaN(errorRate) ? 0 : errorRate,
        display: formatValue(Number.isNaN(errorRate) ? 0 : errorRate, "%"),
        series: matrixToSeries(errorRateSeries).map((p) => ({
          ...p,
          value: Number.isNaN(p.value) ? 0 : p.value,
        })),
        unit: "%",
      },
      activeConnections: {
        current: connections,
        display: formatValue(connections, ""),
        series: matrixToSeries(connectionsSeries),
        unit: "",
      },
      tokenUsage: matrixToTokenUsage(inputTokensSeries, outputTokensSeries),
    };
  } catch (error) {
    console.error("Failed to fetch agent metrics:", error);
    return EMPTY_METRICS;
  }
}

/**
 * Hook to fetch metrics for a specific agent.
 *
 * In demo mode, returns mock data.
 * Otherwise, queries Prometheus for real metrics.
 */
export function useAgentMetrics(agentName: string, namespace: string) {
  const { isDemoMode, loading: demoLoading } = useDemoMode();

  const query = useQuery({
    queryKey: ["agent-metrics", agentName, namespace, isDemoMode],
    queryFn: async () => {
      if (isDemoMode) {
        return getMockMetrics(agentName, namespace);
      }
      return fetchAgentMetrics(agentName, namespace);
    },
    enabled: !demoLoading && !!agentName,
    refetchInterval: isDemoMode ? false : 30000,
    staleTime: 15000,
  });

  return {
    data: query.data ?? EMPTY_METRICS,
    isLoading: query.isLoading || demoLoading,
    error: query.error,
  };
}

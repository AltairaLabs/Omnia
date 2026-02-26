/**
 * Hook to fetch provider-specific usage metrics from Prometheus.
 */

import { useQuery } from "@tanstack/react-query";
import {
  queryPrometheus,
  queryPrometheusRange,
  isPrometheusAvailable,
  type PrometheusMatrixResult,
  type PrometheusVectorResult,
} from "@/lib/prometheus";
import { LLMQueries, LLM_METRICS, buildFilter } from "@/lib/prometheus-queries";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";

export interface ProviderMetricsData {
  /** Whether Prometheus is available */
  available: boolean;
  /** Time series data points for sparklines */
  requestRate: Array<{ timestamp: Date; value: number }>;
  inputTokenRate: Array<{ timestamp: Date; value: number }>;
  outputTokenRate: Array<{ timestamp: Date; value: number }>;
  costRate: Array<{ timestamp: Date; value: number }>;
  /** Current/latest values */
  currentRequestRate: number;
  currentInputTokenRate: number;
  currentOutputTokenRate: number;
  totalCost24h: number;
  totalRequests24h: number;
  totalTokens24h: number;
}

const EMPTY_METRICS: ProviderMetricsData = {
  available: false,
  requestRate: [],
  inputTokenRate: [],
  outputTokenRate: [],
  costRate: [],
  currentRequestRate: 0,
  currentInputTokenRate: 0,
  currentOutputTokenRate: 0,
  totalCost24h: 0,
  totalRequests24h: 0,
  totalTokens24h: 0,
};

/** Prometheus response structure for range queries */
type PrometheusRangeResponse = {
  status: string;
  data?: { result: PrometheusMatrixResult[] };
};

/** Prometheus response structure for instant queries */
type PrometheusInstantResponse = {
  status: string;
  data?: { result: PrometheusVectorResult[] };
};

/**
 * Convert Prometheus matrix response to sparkline data points.
 */
function responseToSparkline(
  response: PrometheusRangeResponse | null
): Array<{ timestamp: Date; value: number }> {
  if (!response || response.status !== "success" || !response.data?.result) {
    return [];
  }

  const result = response.data.result;
  if (result.length === 0) {
    return [];
  }

  // Get the first time series (we're summing so there should only be one)
  const series = result[0];
  if (!series?.values || !Array.isArray(series.values)) {
    return [];
  }

  return series.values.map(([timestamp, value]: [number, string]) => ({
    timestamp: new Date(timestamp * 1000),
    value: Number.parseFloat(value) || 0,
  }));
}

/**
 * Extract scalar value from Prometheus instant query response.
 */
function extractScalar(response: PrometheusInstantResponse | null): number {
  if (!response || response.status !== "success" || !response.data?.result?.length) {
    return 0;
  }
  return Number.parseFloat(response.data.result[0]?.value?.[1] || "0") || 0;
}

/**
 * Fetch usage metrics for a specific provider.
 */
export function useProviderMetrics(providerName: string, providerType?: string) {
  return useQuery({
    queryKey: ["provider-metrics", providerName, providerType],
    queryFn: async (): Promise<ProviderMetricsData> => {
      // We filter by provider type (e.g., "ollama", "anthropic")
      const filter = providerType ? { provider: providerType } : undefined;
      if (!filter) {
        return EMPTY_METRICS;
      }

      // Check if Prometheus is available first
      const available = await isPrometheusAvailable();
      if (!available) {
        return { ...EMPTY_METRICS, available: false };
      }

      const now = new Date();
      const start = new Date(now.getTime() - 24 * 60 * 60 * 1000);
      const step = "15m"; // 15-minute intervals for sparklines

      try {
        // Build filter string for queries
        const filterStr = buildFilter(filter);

        // Query time series for sparklines AND instant totals in parallel
        const [
          requestRateResult,
          inputTokenResult,
          outputTokenResult,
          costResult,
          totalRequestsResult,
          totalInputTokensResult,
          totalOutputTokensResult,
          totalCostResult,
        ] = await Promise.all([
          // Range queries for sparklines
          queryPrometheusRange(
            LLMQueries.requestRate(filter, "5m"),
            start,
            now,
            step
          ).catch(() => null),
          queryPrometheusRange(
            LLMQueries.inputTokenRate(filter, "5m"),
            start,
            now,
            step
          ).catch(() => null),
          queryPrometheusRange(
            LLMQueries.outputTokenRate(filter, "5m"),
            start,
            now,
            step
          ).catch(() => null),
          queryPrometheusRange(
            LLMQueries.costIncrease(filter, "1h"),
            start,
            now,
            step
          ).catch(() => null),
          // Instant queries for 24h totals using increase()
          queryPrometheus(
            `sum(increase(${LLM_METRICS.REQUESTS_TOTAL}{${filterStr}}[24h]))`
          ).catch(() => null),
          queryPrometheus(
            `sum(increase(${LLM_METRICS.INPUT_TOKENS}{${filterStr}}[24h]))`
          ).catch(() => null),
          queryPrometheus(
            `sum(increase(${LLM_METRICS.OUTPUT_TOKENS}{${filterStr}}[24h]))`
          ).catch(() => null),
          queryPrometheus(
            `sum(increase(${LLM_METRICS.COST_USD}{${filterStr}}[24h]))`
          ).catch(() => null),
        ]);

        // Convert Prometheus responses to time series
        const requestRate = responseToSparkline(requestRateResult);
        const inputTokenRate = responseToSparkline(inputTokenResult);
        const outputTokenRate = responseToSparkline(outputTokenResult);
        const costRate = responseToSparkline(costResult);

        // Get current values (last point in each series)
        const currentRequestRate = requestRate[requestRate.length - 1]?.value ?? 0;
        const currentInputTokenRate = inputTokenRate[inputTokenRate.length - 1]?.value ?? 0;
        const currentOutputTokenRate = outputTokenRate[outputTokenRate.length - 1]?.value ?? 0;

        // Extract 24h totals from instant queries
        const totalRequests24h = extractScalar(totalRequestsResult);
        const totalInputTokens24h = extractScalar(totalInputTokensResult);
        const totalOutputTokens24h = extractScalar(totalOutputTokensResult);
        const totalTokens24h = totalInputTokens24h + totalOutputTokens24h;
        const totalCost24h = extractScalar(totalCostResult);

        return {
          available: true,
          requestRate,
          inputTokenRate,
          outputTokenRate,
          costRate,
          currentRequestRate,
          currentInputTokenRate,
          currentOutputTokenRate,
          totalCost24h,
          totalRequests24h,
          totalTokens24h,
        };
      } catch (error) {
        console.warn("Failed to fetch provider metrics:", error);
        return EMPTY_METRICS;
      }
    },
    enabled: !!providerType,
    refetchInterval: 60000, // Refresh every minute
    staleTime: DEFAULT_STALE_TIME,
  });
}

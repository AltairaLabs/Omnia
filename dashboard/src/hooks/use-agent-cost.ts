/**
 * Hook for fetching agent-specific cost metrics from Prometheus.
 *
 * Provides cost data and time series for sparklines on agent cards.
 */

"use client";

import { useQuery } from "@tanstack/react-query";
import {
  queryPrometheus,
  queryPrometheusRange,
  isPrometheusAvailable,
  type PrometheusMatrixResult,
} from "@/lib/prometheus";
import { LLM_METRICS, LABELS } from "@/lib/prometheus-queries";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";

export interface AgentCostData {
  available: boolean;
  totalCost: number;
  inputTokens: number;
  outputTokens: number;
  requests: number;
  /** Time series for sparkline (last 24h, hourly resolution) */
  timeSeries: Array<{ value: number }>;
}

const EMPTY_DATA: AgentCostData = {
  available: false,
  totalCost: 0,
  inputTokens: 0,
  outputTokens: 0,
  requests: 0,
  timeSeries: [],
};

/**
 * Fetch cost data for a specific agent from Prometheus.
 */
async function fetchAgentCost(
  namespace: string,
  agentName: string
): Promise<AgentCostData> {
  const available = await isPrometheusAvailable();
  if (!available) {
    return EMPTY_DATA;
  }

  try {
    const filter = `${LABELS.AGENT}="${agentName}",${LABELS.NAMESPACE}="${namespace}"`;

    const now = new Date();
    const oneDayAgo = new Date(now.getTime() - 24 * 60 * 60 * 1000);

    // Fetch current totals and time series in parallel
    const [costResult, inputResult, outputResult, requestsResult, costSeries] =
      await Promise.all([
        queryPrometheus(`sum(${LLM_METRICS.COST_USD}{${filter}})`),
        queryPrometheus(`sum(${LLM_METRICS.INPUT_TOKENS}{${filter}})`),
        queryPrometheus(`sum(${LLM_METRICS.OUTPUT_TOKENS}{${filter}})`),
        queryPrometheus(`sum(${LLM_METRICS.REQUESTS_TOTAL}{${filter}})`),
        queryPrometheusRange(
          `sum(increase(${LLM_METRICS.COST_USD}{${filter}}[1h]))`,
          oneDayAgo,
          now,
          "1h"
        ),
      ]);

    // Extract scalar values
    const extractScalar = (result: typeof costResult): number => {
      if (result.status !== "success" || !result.data?.result?.length) {
        return 0;
      }
      return Number.parseFloat(result.data.result[0]?.value?.[1] || "0") || 0;
    };

    // Convert time series to sparkline data
    const timeSeries = matrixToSparkline(costSeries);

    return {
      available: true,
      totalCost: extractScalar(costResult),
      inputTokens: extractScalar(inputResult),
      outputTokens: extractScalar(outputResult),
      requests: extractScalar(requestsResult),
      timeSeries,
    };
  } catch (error) {
    console.error(`Failed to fetch cost data for agent ${agentName}:`, error);
    return EMPTY_DATA;
  }
}

/**
 * Convert Prometheus matrix result to sparkline data points.
 * Fills in missing hourly buckets with zeros to ensure a continuous 24-hour line.
 */
function matrixToSparkline(result: {
  status: string;
  data?: { result: PrometheusMatrixResult[] };
}): Array<{ value: number }> {
  if (result.status !== "success" || !result.data?.result?.length) {
    return [];
  }

  // Aggregate all series by timestamp
  const timeMap = new Map<number, number>();

  for (const series of result.data.result) {
    for (const [ts, val] of series.values || []) {
      const value = Number.parseFloat(val) || 0;
      timeMap.set(ts, (timeMap.get(ts) || 0) + value);
    }
  }

  // If no data at all, return empty
  if (timeMap.size === 0) {
    return [];
  }

  // Generate 24 hourly buckets ending at the current hour
  const now = new Date();
  const currentHour = Math.floor(now.getTime() / (60 * 60 * 1000)) * (60 * 60 * 1000);
  const points: Array<{ value: number }> = [];

  for (let i = 23; i >= 0; i--) {
    const bucketTime = currentHour - i * 60 * 60 * 1000;
    const bucketTs = bucketTime / 1000; // Convert to seconds for Prometheus timestamp
    // Look for data within this hour bucket (allow some tolerance)
    let value = 0;
    for (const [ts, v] of timeMap.entries()) {
      if (Math.abs(ts - bucketTs) < 1800) { // Within 30 minutes
        value = v;
        break;
      }
    }
    points.push({ value });
  }

  return points;
}

/**
 * Hook to fetch and cache agent cost data.
 *
 * @param namespace - Agent namespace
 * @param agentName - Agent name
 * @returns Query result with agent cost data
 */
export function useAgentCost(namespace: string, agentName: string) {
  return useQuery({
    queryKey: ["agent-cost", namespace, agentName],
    queryFn: () => fetchAgentCost(namespace, agentName),
    refetchInterval: 60000, // Refresh every minute
    staleTime: DEFAULT_STALE_TIME,
    enabled: Boolean(namespace && agentName),
  });
}

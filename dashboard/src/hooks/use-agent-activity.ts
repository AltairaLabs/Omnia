/**
 * Hook for agent activity metrics from Prometheus.
 *
 * Fetches hourly request counts for the activity chart.
 * Falls back to mock data in demo mode.
 */

import { useQuery } from "@tanstack/react-query";
import {
  queryPrometheusRange,
  isPrometheusAvailable,
  type PrometheusMatrixResult,
} from "@/lib/prometheus";
import { useDemoMode } from "./use-runtime-config";

export interface ActivityDataPoint {
  time: string;
  requests: number;
  sessions: number;
}

/**
 * Generate mock activity data for demo mode.
 */
function generateMockActivityData(): ActivityDataPoint[] {
  const data: ActivityDataPoint[] = [];
  const now = new Date();

  for (let i = 23; i >= 0; i--) {
    const time = new Date(now.getTime() - i * 60 * 60 * 1000);
    const requests = Math.floor(Math.random() * 500) + 200; // NOSONAR - mock data
    const sessions = Math.floor(Math.random() * 100) + 50; // NOSONAR - mock data
    data.push({
      time: time.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
      requests,
      sessions,
    });
  }

  return data;
}

// Cache mock data so it doesn't regenerate on every render
let cachedMockData: ActivityDataPoint[] | null = null;

function getMockActivityData(): ActivityDataPoint[] {
  if (!cachedMockData) {
    cachedMockData = generateMockActivityData();
  }
  return cachedMockData;
}

type PrometheusResult = { status: string; data?: { result: PrometheusMatrixResult[] } };
type ActivityEntry = { requests: number; sessions: number };

/**
 * Process a Prometheus result and update the time map.
 */
function processPrometheusResult(
  result: PrometheusResult,
  timeMap: Map<number, ActivityEntry>,
  field: "requests" | "sessions",
  aggregator: "sum" | "max"
): void {
  if (result.status !== "success" || !result.data?.result) return;

  for (const series of result.data.result) {
    for (const [ts, val] of series.values || []) {
      if (!timeMap.has(ts)) {
        timeMap.set(ts, { requests: 0, sessions: 0 });
      }
      const entry = timeMap.get(ts)!;
      const value = Number.parseFloat(val) || 0;
      entry[field] = aggregator === "sum" ? entry[field] + value : Math.max(entry[field], value);
    }
  }
}

/**
 * Convert Prometheus range result to ActivityDataPoint array.
 */
function matrixToActivityData(
  requestsResult: PrometheusResult,
  sessionsResult: PrometheusResult
): ActivityDataPoint[] {
  const timeMap = new Map<number, ActivityEntry>();

  processPrometheusResult(requestsResult, timeMap, "requests", "sum");
  processPrometheusResult(sessionsResult, timeMap, "sessions", "max");

  return Array.from(timeMap.entries())
    .sort(([a], [b]) => a - b)
    .map(([ts, data]) => ({
      time: new Date(ts * 1000).toLocaleTimeString([], {
        hour: "2-digit",
        minute: "2-digit",
      }),
      requests: Math.round(data.requests),
      sessions: Math.round(data.sessions),
    }));
}

/**
 * Fetch agent activity from Prometheus.
 */
async function fetchAgentActivity(): Promise<{
  available: boolean;
  data: ActivityDataPoint[];
}> {
  // Check availability first
  const available = await isPrometheusAvailable();
  if (!available) {
    return { available: false, data: [] };
  }

  const now = new Date();
  const oneDayAgo = new Date(now.getTime() - 24 * 60 * 60 * 1000);

  try {
    // Fetch requests and sessions in parallel
    const [requestsResult, sessionsResult] = await Promise.all([
      // Total requests per hour (increase over 1h window)
      queryPrometheusRange(
        "sum(increase(omnia_llm_requests_total[1h]))",
        oneDayAgo,
        now,
        "1h"
      ),
      // Active sessions/connections (gauge)
      queryPrometheusRange(
        "sum(omnia_facade_connections_active)",
        oneDayAgo,
        now,
        "1h"
      ),
    ]);

    const data = matrixToActivityData(requestsResult, sessionsResult);

    return {
      available: true,
      data: data.length > 0 ? data : [],
    };
  } catch (error) {
    console.error("Failed to fetch agent activity:", error);
    return { available: false, data: [] };
  }
}

/**
 * Hook to fetch agent activity data.
 *
 * In demo mode, returns mock data.
 * Otherwise, queries Prometheus for real metrics.
 */
export function useAgentActivity() {
  const { isDemoMode, loading: demoLoading } = useDemoMode();

  const query = useQuery({
    queryKey: ["agent-activity", isDemoMode],
    queryFn: async () => {
      // In demo mode, return mock data
      if (isDemoMode) {
        return {
          available: true,
          data: getMockActivityData(),
          isDemo: true,
        };
      }

      // Otherwise fetch from Prometheus
      const result = await fetchAgentActivity();
      return {
        ...result,
        isDemo: false,
      };
    },
    enabled: !demoLoading,
    refetchInterval: isDemoMode ? false : 60000, // Refresh every minute in live mode
    staleTime: 30000,
  });

  return {
    data: query.data?.data ?? [],
    available: query.data?.available ?? false,
    isDemo: query.data?.isDemo ?? isDemoMode,
    isLoading: query.isLoading || demoLoading,
    error: query.error,
  };
}

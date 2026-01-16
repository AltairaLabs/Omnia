"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";
import { queryPrometheus, isPrometheusAvailable } from "@/lib/prometheus";
import { AgentQueries } from "@/lib/prometheus-queries";

// Stats type with required nested objects for dashboard display
export interface DashboardStats {
  agents: {
    total: number;
    running: number;
    pending: number;
    failed: number;
  };
  promptPacks: {
    total: number;
    active: number;
    canary: number;
  };
  tools: {
    total: number;
    available: number;
    degraded: number;
  };
  sessions: {
    active: number;
    trend: number | null; // Percentage change from 1 hour ago, null if unavailable
  };
}

export function useStats() {
  const service = useDataService();

  return useQuery({
    queryKey: ["stats", service.name],
    queryFn: async (): Promise<DashboardStats> => {
      const stats = await service.getStats();

      // Fetch session metrics from Prometheus
      let sessionActive = 0;
      let sessionTrend: number | null = null;

      const prometheusAvailable = await isPrometheusAvailable();
      if (prometheusAvailable) {
        try {
          // Get current active sessions
          const currentResult = await queryPrometheus(AgentQueries.activeSessions());
          if (currentResult.status === "success" && currentResult.data?.result?.[0]) {
            sessionActive = Number.parseFloat(currentResult.data.result[0].value[1]) || 0;
          }

          // Get sessions from 1 hour ago using offset
          const pastResult = await queryPrometheus(
            `sum(omnia_agent_sessions_active offset 1h)`
          );
          if (pastResult.status === "success" && pastResult.data?.result?.[0]) {
            const pastValue = Number.parseFloat(pastResult.data.result[0].value[1]) || 0;
            if (pastValue > 0) {
              sessionTrend = ((sessionActive - pastValue) / pastValue) * 100;
            } else if (sessionActive > 0) {
              sessionTrend = 100; // Went from 0 to some value
            } else {
              sessionTrend = 0; // Both are 0
            }
          }
        } catch (error) {
          console.warn("Failed to fetch session metrics:", error);
        }
      }

      // Normalize stats with defaults and add sessions count
      return {
        agents: {
          total: stats.agents?.total ?? 0,
          running: stats.agents?.running ?? 0,
          pending: stats.agents?.pending ?? 0,
          failed: stats.agents?.failed ?? 0,
        },
        promptPacks: {
          total: stats.promptPacks?.total ?? 0,
          active: stats.promptPacks?.active ?? 0,
          canary: stats.promptPacks?.canary ?? 0,
        },
        tools: {
          total: stats.tools?.total ?? 0,
          available: stats.tools?.available ?? 0,
          degraded: stats.tools?.degraded ?? 0,
        },
        sessions: {
          active: sessionActive,
          trend: sessionTrend,
        },
      };
    },
    refetchInterval: 30000, // Refetch every 30 seconds
  });
}

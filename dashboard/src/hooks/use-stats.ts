"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";

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
  };
}

export function useStats() {
  const service = useDataService();

  return useQuery({
    queryKey: ["stats", service.name],
    queryFn: async (): Promise<DashboardStats> => {
      const stats = await service.getStats();
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
        sessions: { active: 0 },
      };
    },
    refetchInterval: 30000, // Refetch every 30 seconds
  });
}

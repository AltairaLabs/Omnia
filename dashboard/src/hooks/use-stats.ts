"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchStats, type Stats } from "@/lib/api-client";

export interface DashboardStats extends Stats {
  sessions: {
    active: number;
  };
}

export function useStats() {
  return useQuery({
    queryKey: ["stats"],
    queryFn: async (): Promise<DashboardStats> => {
      const stats = await fetchStats();
      // Add sessions count (not tracked by operator, would come from session store)
      return {
        ...stats,
        sessions: { active: 0 }, // TODO: Get from session API when available
      };
    },
    refetchInterval: 30000, // Refetch every 30 seconds
  });
}

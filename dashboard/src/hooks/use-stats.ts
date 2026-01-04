"use client";

import { useQuery } from "@tanstack/react-query";
import { getMockStats } from "@/lib/mock-data";

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
  return useQuery({
    queryKey: ["stats"],
    queryFn: async (): Promise<DashboardStats> => {
      // Simulate network delay
      await new Promise((resolve) => setTimeout(resolve, 200));
      return getMockStats();
    },
    refetchInterval: 30000, // Refetch every 30 seconds
  });
}

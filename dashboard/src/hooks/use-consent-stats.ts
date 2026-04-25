"use client";

import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import { MemoryApiService } from "@/lib/data/memory-api-service";
import type { ConsentStats } from "@/lib/memory-analytics/types";

const EMPTY_STATS: ConsentStats = {
  totalUsers: 0,
  optedOutAll: 0,
  grantsByCategory: {},
};

/**
 * Fetch the workspace-level consent posture stats.
 * Returns the empty-stats sentinel when no workspace is selected.
 */
export function useConsentStats() {
  const { currentWorkspace } = useWorkspace();

  return useQuery({
    queryKey: ["consent-stats", currentWorkspace?.name],
    queryFn: async (): Promise<ConsentStats> => {
      if (!currentWorkspace) return EMPTY_STATS;
      const service = new MemoryApiService();
      return service.getConsentStats(currentWorkspace.name);
    },
    enabled: !!currentWorkspace,
    refetchInterval: 5 * 60 * 1000,
  });
}

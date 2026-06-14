"use client";

import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import { MemoryApiService } from "@/lib/data/memory-api-service";
import type { EnforcementStats } from "@/lib/memory-analytics/types";

const EMPTY_STATS: EnforcementStats = {
  piiBlocked: 0,
  redactions: 0,
};

/**
 * Fetch the workspace-level privacy enforcement stats — counts of opt-out
 * write blocks (piiBlocked) and PII redactions (redactions).
 * Returns the empty-stats sentinel when no workspace is selected.
 */
export function useEnforcementStats() {
  const { currentWorkspace } = useWorkspace();

  return useQuery({
    queryKey: ["enforcement-stats", currentWorkspace?.name],
    queryFn: async (): Promise<EnforcementStats> => {
      if (!currentWorkspace) return EMPTY_STATS;
      const service = new MemoryApiService();
      return service.getEnforcementStats(currentWorkspace.name);
    },
    enabled: !!currentWorkspace,
    refetchInterval: 5 * 60 * 1000,
  });
}

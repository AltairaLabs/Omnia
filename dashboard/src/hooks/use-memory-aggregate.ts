"use client";

import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import { MemoryApiService } from "@/lib/data/memory-api-service";
import type {
  AggregateRow,
  MemoryAggregateOptions,
} from "@/lib/memory-analytics/types";

export type UseMemoryAggregateOptions = MemoryAggregateOptions & {
  enabled?: boolean;
};

/**
 * Fetch a memory aggregate row set for the current workspace.
 * Returns an empty array when no workspace is selected.
 */
export function useMemoryAggregate(options: UseMemoryAggregateOptions) {
  const { currentWorkspace } = useWorkspace();
  const { groupBy, metric, from, to, limit, enabled = true } = options;

  return useQuery({
    queryKey: [
      "memory-aggregate",
      currentWorkspace?.name,
      groupBy,
      metric ?? "count",
      from ?? "",
      to ?? "",
      limit ?? 0,
    ],
    queryFn: async (): Promise<AggregateRow[]> => {
      if (!currentWorkspace) return [];
      const service = new MemoryApiService();
      return service.getMemoryAggregate({
        workspace: currentWorkspace.name,
        groupBy,
        metric,
        from,
        to,
        limit,
      });
    },
    enabled: enabled && !!currentWorkspace,
    refetchInterval: 5 * 60 * 1000,
  });
}

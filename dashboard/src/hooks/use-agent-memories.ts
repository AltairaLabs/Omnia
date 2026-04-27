"use client";

import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import { MemoryApiService } from "@/lib/data/memory-api-service";
import type { MemoryEntity } from "@/lib/data/types";

interface UseAgentMemoriesOptions {
  agentId: string | undefined;
  enabled?: boolean;
}

/**
 * Fetch the (workspace, agent)-scoped memory rows for the given agent.
 * Returns an empty list when no workspace is selected or `agentId` is missing.
 */
export function useAgentMemories({ agentId, enabled = true }: UseAgentMemoriesOptions) {
  const { currentWorkspace } = useWorkspace();

  return useQuery({
    queryKey: ["agent-memories", currentWorkspace?.name, agentId],
    queryFn: async (): Promise<{ memories: MemoryEntity[]; total: number }> => {
      if (!currentWorkspace || !agentId) {
        return { memories: [], total: 0 };
      }
      const service = new MemoryApiService();
      return service.getAgentMemories(currentWorkspace.name, agentId);
    },
    enabled: enabled && !!currentWorkspace && !!agentId,
    refetchInterval: 60 * 1000,
  });
}

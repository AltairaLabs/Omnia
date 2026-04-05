"use client";

import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import { MemoryApiService } from "@/lib/data/memory-api-service";
import type {
  MemoryEntity,
  MemoryListOptions,
  MemorySearchOptions,
} from "@/lib/data/types";

/**
 * Fetch memories for the current workspace with optional filters.
 *
 * Pass `enabled: false` to suppress the fetch (e.g. for anonymous users,
 * where the memory-api would reject the request for lack of a user scope).
 */
export function useMemories(
  options?: Omit<Partial<MemoryListOptions>, "workspace"> & { enabled?: boolean }
) {
  const { currentWorkspace } = useWorkspace();

  const { userId, type, purpose, limit, offset, enabled = true } = options ?? {};

  return useQuery({
    queryKey: ["memories", currentWorkspace?.name, userId, type, purpose, limit, offset],
    queryFn: async () => {
      if (!currentWorkspace) {
        return { memories: [] as MemoryEntity[], total: 0 };
      }
      const service = new MemoryApiService();
      return service.getMemories({
        workspace: currentWorkspace.name,
        userId,
        type,
        purpose,
        limit,
        offset,
      });
    },
    enabled: !!currentWorkspace && enabled,
    staleTime: 30000,
  });
}

/**
 * Search memories for the current workspace.
 * Only fires when query is non-empty.
 */
export function useMemorySearch(
  query: string,
  options?: Omit<Partial<MemorySearchOptions>, "workspace" | "query">
) {
  const { currentWorkspace } = useWorkspace();

  const { userId, type, purpose, limit, offset, minConfidence } = options ?? {};

  return useQuery({
    queryKey: ["memories-search", currentWorkspace?.name, query, userId, type, purpose, limit, offset, minConfidence],
    queryFn: async () => {
      if (!currentWorkspace) {
        return { memories: [] as MemoryEntity[], total: 0 };
      }
      const service = new MemoryApiService();
      return service.searchMemories({
        workspace: currentWorkspace.name,
        query,
        userId,
        type,
        purpose,
        limit,
        offset,
        minConfidence,
      });
    },
    enabled: !!currentWorkspace && !!query,
    staleTime: 30000,
  });
}

/**
 * Export all memories for a user in the current workspace.
 */
export function useMemoryExport(userId: string) {
  const { currentWorkspace } = useWorkspace();

  return useQuery({
    queryKey: ["memories-export", currentWorkspace?.name, userId],
    queryFn: async () => {
      if (!currentWorkspace) {
        return [] as MemoryEntity[];
      }
      const service = new MemoryApiService();
      return service.exportMemories(currentWorkspace.name, userId);
    },
    enabled: !!currentWorkspace && !!userId,
    staleTime: 30000,
  });
}

"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import {
  InstitutionalMemoryService,
  type InstitutionalCreateInput,
} from "@/lib/data/institutional-memory-service";
import type { MemoryEntity } from "@/lib/data/types";

const institutionalMemoriesKey = (workspace: string | undefined) =>
  ["institutional-memories", workspace] as const;

/**
 * Fetch workspace-scoped institutional memories.
 */
export function useInstitutionalMemories(options?: { limit?: number; offset?: number; enabled?: boolean }) {
  const { currentWorkspace } = useWorkspace();
  const enabled = options?.enabled ?? true;
  return useQuery({
    queryKey: institutionalMemoriesKey(currentWorkspace?.name),
    queryFn: async () => {
      if (!currentWorkspace) {
        return { memories: [] as MemoryEntity[], total: 0 };
      }
      const service = new InstitutionalMemoryService();
      return service.list({
        workspace: currentWorkspace.name,
        limit: options?.limit,
        offset: options?.offset,
      });
    },
    enabled: !!currentWorkspace && enabled,
    staleTime: 30000,
  });
}

/**
 * Create a new institutional memory. Invalidates the list on success so the
 * new entry appears without a manual refetch.
 */
export function useCreateInstitutionalMemory() {
  const { currentWorkspace } = useWorkspace();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: Omit<InstitutionalCreateInput, "workspace">) => {
      if (!currentWorkspace) {
        throw new Error("No workspace selected");
      }
      const service = new InstitutionalMemoryService();
      return service.create({ ...input, workspace: currentWorkspace.name });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: institutionalMemoriesKey(currentWorkspace?.name) });
    },
  });
}

/**
 * Soft-delete an institutional memory by ID.
 */
export function useDeleteInstitutionalMemory() {
  const { currentWorkspace } = useWorkspace();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (memoryId: string) => {
      if (!currentWorkspace) {
        throw new Error("No workspace selected");
      }
      const service = new InstitutionalMemoryService();
      await service.delete(currentWorkspace.name, memoryId);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: institutionalMemoriesKey(currentWorkspace?.name) });
    },
  });
}

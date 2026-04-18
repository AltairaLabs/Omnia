"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import { useAuth } from "@/hooks/use-auth";
import { MemoryApiService } from "@/lib/data/memory-api-service";

const service = new MemoryApiService();

/**
 * Delete a single memory by ID.
 * Invalidates the memories query cache on success.
 */
export function useDeleteMemory() {
  const { currentWorkspace } = useWorkspace();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (memoryId: string) => {
      if (!currentWorkspace) throw new Error("No workspace selected");
      return service.deleteMemory(currentWorkspace.name, memoryId);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["memories"] });
    },
  });
}

/**
 * Delete all memories for the current user.
 * Invalidates the memories query cache on success.
 */
export function useDeleteAllMemories() {
  const { currentWorkspace } = useWorkspace();
  const { user } = useAuth();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async () => {
      if (!currentWorkspace || !user?.id) throw new Error("No workspace or user");
      return service.deleteAllMemories(currentWorkspace.name, user.id);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["memories"] });
    },
  });
}

function triggerDownload(memories: object[], filename: string): void {
  const blob = new Blob([JSON.stringify(memories, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

/**
 * Export all memories as a JSON download.
 * Returns a mutation that triggers the browser download.
 */
export function useExportMemories() {
  const { currentWorkspace } = useWorkspace();
  const { user } = useAuth();

  return useMutation({
    mutationFn: async () => {
      if (!currentWorkspace || !user?.id) throw new Error("No workspace or user");
      const memories = await service.exportMemories(currentWorkspace.name, user.id);
      const filename = `memories-export-${new Date().toISOString().split("T")[0]}.json`;
      triggerDownload(memories, filename);
      return memories;
    },
  });
}

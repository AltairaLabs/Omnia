"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService, type ToolRegistry as ServiceToolRegistry } from "@/lib/data";
import { useWorkspace } from "@/contexts/workspace-context";
import type { ToolRegistry, ToolRegistryPhase } from "@/types";

interface UseToolRegistriesOptions {
  phase?: ToolRegistryPhase;
}

/**
 * Fetch workspace-scoped tool registries.
 * Uses current workspace context.
 * The DataService handles whether to use mock data (demo mode) or real API (live mode).
 */
export function useToolRegistries(options: UseToolRegistriesOptions = {}) {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();
  const { phase } = options;

  return useQuery({
    queryKey: ["toolRegistries", currentWorkspace?.name, phase, service.name],
    queryFn: async (): Promise<ToolRegistry[]> => {
      if (!currentWorkspace) {
        return [];
      }

      // DataService handles demo vs live mode internally
      let registries = await service.getToolRegistries(currentWorkspace.name) as unknown as ToolRegistry[];

      // Client-side filtering for phase
      if (phase) {
        registries = registries.filter((r) => r.status?.phase === phase);
      }

      return registries;
    },
    enabled: !!currentWorkspace,
    staleTime: 30000, // 30 seconds
    refetchOnMount: true, // Only refetch if stale
  });
}

/**
 * Fetch a single workspace-scoped tool registry by name.
 * Uses current workspace context.
 *
 * @param name - Tool registry name
 * @param _namespace - Deprecated parameter, kept for backwards compatibility.
 */
export function useToolRegistry(name: string, _namespace?: string) {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();

  return useQuery({
    queryKey: ["toolRegistry", currentWorkspace?.name, name, service.name],
    queryFn: async (): Promise<ToolRegistry | null> => {
      if (!currentWorkspace) {
        return null;
      }

      // DataService handles demo vs live mode internally
      const registry = await service.getToolRegistry(currentWorkspace.name, name) as ServiceToolRegistry | undefined;
      return (registry as unknown as ToolRegistry) || null;
    },
    enabled: !!name && !!currentWorkspace,
    staleTime: 30000, // 30 seconds
    refetchOnMount: true, // Only refetch if stale
  });
}

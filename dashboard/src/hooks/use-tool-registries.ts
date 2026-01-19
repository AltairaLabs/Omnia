"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService, type ToolRegistry as ServiceToolRegistry } from "@/lib/data";
import type { ToolRegistry, ToolRegistryPhase } from "@/types";

interface UseToolRegistriesOptions {
  phase?: ToolRegistryPhase;
}

/**
 * Fetch shared tool registries.
 * Tool registries are shared/system-wide resources, not workspace-scoped.
 * The DataService handles whether to use mock data (demo mode) or real API (live mode).
 */
export function useToolRegistries(options: UseToolRegistriesOptions = {}) {
  const service = useDataService();

  return useQuery({
    queryKey: ["toolRegistries", options, service.name],
    queryFn: async (): Promise<ToolRegistry[]> => {
      // DataService handles demo vs live mode internally
      let registries = await service.getToolRegistries() as unknown as ToolRegistry[];

      // Client-side filtering for phase
      if (options.phase) {
        registries = registries.filter((r) => r.status?.phase === options.phase);
      }

      return registries;
    },
  });
}

/**
 * Fetch a single tool registry by name.
 * Tool registries are shared/system-wide resources, not workspace-scoped.
 *
 * @param name - Tool registry name
 * @param _namespace - Deprecated parameter, kept for backwards compatibility.
 */
export function useToolRegistry(name: string, _namespace?: string) {
  const service = useDataService();

  return useQuery({
    queryKey: ["toolRegistry", name, service.name],
    queryFn: async (): Promise<ToolRegistry | null> => {
      // DataService handles demo vs live mode internally
      const registry = await service.getToolRegistry(name) as ServiceToolRegistry | undefined;
      return (registry as unknown as ToolRegistry) || null;
    },
    enabled: !!name,
  });
}

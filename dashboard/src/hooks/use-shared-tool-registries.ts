"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService, type ToolRegistry as ServiceToolRegistry } from "@/lib/data";
import type { ToolRegistry } from "@/types";

/**
 * Fetches shared tool registries from the system namespace.
 * Shared tool registries are available to all workspaces (read-only).
 */
export function useSharedToolRegistries() {
  const service = useDataService();

  return useQuery({
    queryKey: ["shared-tool-registries", service.name],
    queryFn: async (): Promise<ToolRegistry[]> => {
      const registries = await service.getSharedToolRegistries();
      return registries as unknown as ToolRegistry[];
    },
    staleTime: 30000, // 30 seconds
    refetchOnMount: true, // Only refetch if stale
  });
}

/**
 * Fetches a single shared tool registry by name.
 */
export function useSharedToolRegistry(name: string) {
  const service = useDataService();

  return useQuery({
    queryKey: ["shared-tool-registry", name, service.name],
    queryFn: async (): Promise<ToolRegistry | null> => {
      const registry = await service.getSharedToolRegistry(name) as ServiceToolRegistry | undefined;
      return (registry as unknown as ToolRegistry) || null;
    },
    enabled: !!name,
    staleTime: 30000, // 30 seconds
    refetchOnMount: true, // Only refetch if stale
  });
}

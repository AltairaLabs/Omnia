"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";
import type { ToolRegistry, ToolRegistryPhase } from "@/types";

interface UseToolRegistriesOptions {
  namespace?: string;
  phase?: ToolRegistryPhase;
}

export function useToolRegistries(options: UseToolRegistriesOptions = {}) {
  const service = useDataService();

  return useQuery({
    queryKey: ["toolRegistries", options, service.name],
    queryFn: async (): Promise<ToolRegistry[]> => {
      const response = await service.getToolRegistries(options.namespace);
      let registries = response as unknown as ToolRegistry[];

      // Client-side filtering for phase
      if (options.phase) {
        registries = registries.filter((r) => r.status?.phase === options.phase);
      }

      return registries;
    },
  });
}

export function useToolRegistry(name: string, namespace: string = "production") {
  const service = useDataService();

  return useQuery({
    queryKey: ["toolRegistry", namespace, name, service.name],
    queryFn: async (): Promise<ToolRegistry | null> => {
      const response = await service.getToolRegistry(namespace, name);
      return (response as unknown as ToolRegistry) || null;
    },
    enabled: !!name,
  });
}

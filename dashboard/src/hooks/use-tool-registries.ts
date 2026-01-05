"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchToolRegistries, fetchToolRegistry } from "@/lib/api/client";
import type { ToolRegistry, ToolRegistryPhase } from "@/types";

interface UseToolRegistriesOptions {
  namespace?: string;
  phase?: ToolRegistryPhase;
}

export function useToolRegistries(options: UseToolRegistriesOptions = {}) {
  return useQuery({
    queryKey: ["toolRegistries", options],
    queryFn: async (): Promise<ToolRegistry[]> => {
      const response = await fetchToolRegistries(options.namespace);
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
  return useQuery({
    queryKey: ["toolRegistry", namespace, name],
    queryFn: async (): Promise<ToolRegistry | null> => {
      const response = await fetchToolRegistry(namespace, name);
      return (response as unknown as ToolRegistry) || null;
    },
    enabled: !!name,
  });
}

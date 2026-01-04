"use client";

import { useQuery } from "@tanstack/react-query";
import { mockToolRegistries } from "@/lib/mock-data";
import type { ToolRegistry, ToolRegistryPhase } from "@/types";

interface UseToolRegistriesOptions {
  namespace?: string;
  phase?: ToolRegistryPhase;
}

export function useToolRegistries(options: UseToolRegistriesOptions = {}) {
  return useQuery({
    queryKey: ["toolRegistries", options],
    queryFn: async (): Promise<ToolRegistry[]> => {
      // Simulate network delay
      await new Promise((resolve) => setTimeout(resolve, 300));

      let registries = [...mockToolRegistries];

      if (options.namespace) {
        registries = registries.filter(
          (r) => r.metadata.namespace === options.namespace
        );
      }

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
      // Simulate network delay
      await new Promise((resolve) => setTimeout(resolve, 200));

      const registry = mockToolRegistries.find(
        (r) => r.metadata.name === name && r.metadata.namespace === namespace
      );

      return registry || null;
    },
    enabled: !!name,
  });
}

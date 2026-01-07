"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";
import type { PromptPack, PromptPackPhase } from "@/types";

interface UsePromptPacksOptions {
  namespace?: string;
  phase?: PromptPackPhase;
}

export function usePromptPacks(options: UsePromptPacksOptions = {}) {
  const service = useDataService();

  return useQuery({
    queryKey: ["promptPacks", options, service.name],
    queryFn: async (): Promise<PromptPack[]> => {
      const response = await service.getPromptPacks(options.namespace);
      let packs = response as unknown as PromptPack[];

      // Client-side filtering for phase
      if (options.phase) {
        packs = packs.filter((p) => p.status?.phase === options.phase);
      }

      return packs;
    },
  });
}

export function usePromptPack(name: string, namespace: string = "production") {
  const service = useDataService();

  return useQuery({
    queryKey: ["promptPack", namespace, name, service.name],
    queryFn: async (): Promise<PromptPack | null> => {
      const response = await service.getPromptPack(namespace, name);
      return (response as unknown as PromptPack) || null;
    },
    enabled: !!name,
  });
}

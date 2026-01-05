"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchPromptPacks, fetchPromptPack } from "@/lib/api/client";
import type { PromptPack, PromptPackPhase } from "@/types";

interface UsePromptPacksOptions {
  namespace?: string;
  phase?: PromptPackPhase;
}

export function usePromptPacks(options: UsePromptPacksOptions = {}) {
  return useQuery({
    queryKey: ["promptPacks", options],
    queryFn: async (): Promise<PromptPack[]> => {
      const response = await fetchPromptPacks(options.namespace);
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
  return useQuery({
    queryKey: ["promptPack", namespace, name],
    queryFn: async (): Promise<PromptPack | null> => {
      const response = await fetchPromptPack(namespace, name);
      return (response as unknown as PromptPack) || null;
    },
    enabled: !!name,
  });
}

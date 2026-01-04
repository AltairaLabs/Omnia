"use client";

import { useQuery } from "@tanstack/react-query";
import { mockPromptPacks } from "@/lib/mock-data";
import type { PromptPack, PromptPackPhase } from "@/types";

interface UsePromptPacksOptions {
  namespace?: string;
  phase?: PromptPackPhase;
}

export function usePromptPacks(options: UsePromptPacksOptions = {}) {
  return useQuery({
    queryKey: ["promptPacks", options],
    queryFn: async (): Promise<PromptPack[]> => {
      // Simulate network delay
      await new Promise((resolve) => setTimeout(resolve, 300));

      let packs = [...mockPromptPacks];

      if (options.namespace) {
        packs = packs.filter((p) => p.metadata.namespace === options.namespace);
      }

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
      // Simulate network delay
      await new Promise((resolve) => setTimeout(resolve, 200));

      const pack = mockPromptPacks.find(
        (p) => p.metadata.name === name && p.metadata.namespace === namespace
      );

      return pack || null;
    },
    enabled: !!name,
  });
}

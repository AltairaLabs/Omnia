"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService, type PromptPack as ServicePromptPack } from "@/lib/data";
import { useWorkspace } from "@/contexts/workspace-context";
import type { PromptPack, PromptPackPhase } from "@/types";

interface UsePromptPacksOptions {
  phase?: PromptPackPhase;
}

/**
 * Fetch prompt packs for the current workspace.
 * The DataService handles whether to use mock data (demo mode) or real API (live mode).
 */
export function usePromptPacks(options: UsePromptPacksOptions = {}) {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();

  return useQuery({
    queryKey: ["promptPacks", currentWorkspace?.name, options, service.name],
    queryFn: async (): Promise<PromptPack[]> => {
      if (!currentWorkspace) {
        return [];
      }

      // DataService handles demo vs live mode internally
      let packs = await service.getPromptPacks(currentWorkspace.name) as unknown as PromptPack[];

      // Client-side filtering for phase
      if (options.phase) {
        packs = packs.filter((p) => p.status?.phase === options.phase);
      }

      return packs;
    },
    enabled: !!currentWorkspace,
  });
}

/**
 * Fetch a single prompt pack by name.
 * Uses current workspace context.
 *
 * @param name - Prompt pack name
 * @param _namespace - Deprecated parameter, kept for backwards compatibility. Use workspace context instead.
 */
export function usePromptPack(name: string, _namespace?: string) {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();

  return useQuery({
    queryKey: ["promptPack", currentWorkspace?.name, name, service.name],
    queryFn: async (): Promise<PromptPack | null> => {
      if (!currentWorkspace) {
        return null;
      }

      // DataService handles demo vs live mode internally
      const pack = await service.getPromptPack(currentWorkspace.name, name) as ServicePromptPack | undefined;
      return (pack as unknown as PromptPack) || null;
    },
    enabled: !!name && !!currentWorkspace,
  });
}

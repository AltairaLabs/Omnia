"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";
import type { PromptPackContent } from "@/lib/data/types";

/**
 * Hook to fetch the resolved content (pack.json) of a PromptPack.
 * Returns the parsed content from the ConfigMap referenced by the PromptPack.
 */
export function usePromptPackContent(name: string, namespace: string = "default") {
  const service = useDataService();

  return useQuery({
    queryKey: ["promptPackContent", namespace, name, service.name],
    queryFn: async (): Promise<PromptPackContent | null> => {
      const response = await service.getPromptPackContent(namespace, name);
      return response || null;
    },
    enabled: !!name,
  });
}

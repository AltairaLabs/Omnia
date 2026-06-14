"use client";

import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import type { GalaxyResponse } from "@/lib/memory-galaxy/types";

export function useMemoryProjection(options?: { enabled?: boolean }) {
  const { currentWorkspace } = useWorkspace();
  const enabled = (options?.enabled ?? true) && !!currentWorkspace;

  return useQuery({
    queryKey: ["memory-projection", currentWorkspace?.name],
    queryFn: async (): Promise<GalaxyResponse> => {
      const res = await fetch(
        `/api/workspaces/${encodeURIComponent(currentWorkspace!.name)}/memory/projection`,
      );
      if (!res.ok) throw new Error(`Projection request failed: ${res.status}`);
      return res.json();
    },
    enabled,
    staleTime: 5 * 60 * 1000,
  });
}

"use client";

import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import type { GalaxyResponse } from "@/lib/memory-galaxy/types";

// While a large workspace is still being pre-rendered the backend returns
// status:"pending"; re-poll at this cadence until it flips to "ready".
const PENDING_POLL_MS = 2000;

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
    // Poll while the worker is still rendering; stop once a layout is ready.
    // refetchInterval fires on its own timer regardless of staleTime.
    refetchInterval: (query) =>
      query.state.data?.status === "pending" ? PENDING_POLL_MS : false,
    // Once ready, a galaxy is stable — don't refetch on remount for 5 min.
    staleTime: 5 * 60 * 1000,
  });
}

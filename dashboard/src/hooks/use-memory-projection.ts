"use client";

import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import { useDemoMode } from "@/hooks/core";
import { generateMockProjection } from "@/lib/memory-galaxy/mock-projection";
import type { GalaxyResponse } from "@/lib/memory-galaxy/types";

// While a large workspace is still being pre-rendered the backend returns
// status:"pending"; re-poll at this cadence until it flips to "ready".
const PENDING_POLL_MS = 2000;

// Deterministic offline galaxy for demo mode (cluster-free theming preview).
const DEMO_PROJECTION = { seed: 42, count: 120 };

export function useMemoryProjection(options?: { enabled?: boolean }) {
  const { currentWorkspace } = useWorkspace();
  const { isDemoMode } = useDemoMode();
  // In demo mode the galaxy is served from a deterministic mock, so it needs no
  // real workspace and works with the operator/memory-api offline.
  const enabled = (options?.enabled ?? true) && (isDemoMode || !!currentWorkspace);

  return useQuery({
    queryKey: ["memory-projection", isDemoMode ? "demo" : currentWorkspace?.name],
    queryFn: async (): Promise<GalaxyResponse> => {
      if (isDemoMode) {
        return generateMockProjection(DEMO_PROJECTION);
      }
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

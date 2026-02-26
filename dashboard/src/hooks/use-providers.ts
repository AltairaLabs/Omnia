"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService, type Provider as ServiceProvider } from "@/lib/data";
import { useWorkspace } from "@/contexts/workspace-context";
import type { Provider } from "@/types";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";

type ProviderPhase = "Ready" | "Error";

interface UseProvidersOptions {
  phase?: ProviderPhase;
}

/**
 * Fetch workspace-scoped providers.
 * Uses current workspace context.
 * The DataService handles whether to use mock data (demo mode) or real API (live mode).
 */
export function useProviders(options: UseProvidersOptions = {}) {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();
  const { phase } = options;

  return useQuery({
    queryKey: ["providers", currentWorkspace?.name, phase, service.name],
    queryFn: async (): Promise<Provider[]> => {
      if (!currentWorkspace) {
        return [];
      }

      let providers = await service.getProviders(currentWorkspace.name) as unknown as Provider[];

      // Client-side filtering for phase
      if (phase) {
        providers = providers.filter((p) => p.status?.phase === phase);
      }

      return providers;
    },
    enabled: !!currentWorkspace,
    // Reasonable stale time to prevent duplicate fetches during component re-renders
    staleTime: DEFAULT_STALE_TIME,
    refetchOnMount: true, // Only refetch if stale
  });
}

/**
 * Fetch a single workspace-scoped provider by name.
 * Uses current workspace context.
 *
 * @param name - Provider name (can be undefined)
 * @param _namespace - Deprecated parameter, kept for backwards compatibility.
 */
export function useProvider(name: string | undefined, _namespace?: string) {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();

  return useQuery({
    queryKey: ["provider", currentWorkspace?.name, name, service.name],
    queryFn: async (): Promise<Provider | null> => {
      if (!name || !currentWorkspace) return null;
      const provider = await service.getProvider(currentWorkspace.name, name) as ServiceProvider | undefined;
      return (provider as unknown as Provider) || null;
    },
    enabled: !!name && !!currentWorkspace,
    // Reasonable stale time to prevent duplicate fetches during component re-renders
    staleTime: DEFAULT_STALE_TIME,
    refetchOnMount: true, // Only refetch if stale
  });
}

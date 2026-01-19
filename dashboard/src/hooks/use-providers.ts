"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService, type Provider as ServiceProvider } from "@/lib/data";
import type { Provider } from "@/types";

type ProviderPhase = "Ready" | "Error";

interface UseProvidersOptions {
  phase?: ProviderPhase;
}

/**
 * Fetch shared providers.
 * Providers are shared/system-wide resources, not workspace-scoped.
 * The DataService handles whether to use mock data (demo mode) or real API (live mode).
 */
export function useProviders(options: UseProvidersOptions = {}) {
  const service = useDataService();

  return useQuery({
    queryKey: ["providers", options, service.name],
    queryFn: async (): Promise<Provider[]> => {
      const response = await service.getProviders();
      let providers = response as unknown as Provider[];

      // Client-side filtering for phase
      if (options.phase) {
        providers = providers.filter((p) => p.status?.phase === options.phase);
      }

      return providers;
    },
  });
}

/**
 * Fetch a single provider by name.
 * Providers are shared/system-wide resources, not workspace-scoped.
 *
 * @param name - Provider name (can be undefined)
 * @param _namespace - Deprecated parameter, kept for backwards compatibility.
 */
export function useProvider(name: string | undefined, _namespace?: string) {
  const service = useDataService();

  return useQuery({
    queryKey: ["provider", name, service.name],
    queryFn: async (): Promise<Provider | null> => {
      if (!name) return null;
      const response = await service.getProvider(name) as ServiceProvider | undefined;
      return (response as unknown as Provider) || null;
    },
    enabled: !!name,
  });
}

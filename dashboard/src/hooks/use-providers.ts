"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";
import type { Provider } from "@/types";

type ProviderPhase = "Ready" | "Error";

interface UseProvidersOptions {
  namespace?: string;
  phase?: ProviderPhase;
}

export function useProviders(options: UseProvidersOptions = {}) {
  const service = useDataService();

  return useQuery({
    queryKey: ["providers", options, service.name],
    queryFn: async (): Promise<Provider[]> => {
      const response = await service.getProviders(options.namespace);
      let providers = response as unknown as Provider[];

      // Client-side filtering for phase
      if (options.phase) {
        providers = providers.filter((p) => p.status?.phase === options.phase);
      }

      return providers;
    },
  });
}

export function useProvider(name: string, namespace: string = "default") {
  const service = useDataService();

  return useQuery({
    queryKey: ["provider", namespace, name, service.name],
    queryFn: async (): Promise<Provider | null> => {
      const response = await service.getProvider(namespace, name);
      return (response as unknown as Provider) || null;
    },
    enabled: !!name,
  });
}

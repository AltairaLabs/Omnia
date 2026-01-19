"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService, type Provider as ServiceProvider } from "@/lib/data";
import type { Provider } from "@/types";

/**
 * Fetches shared providers from the system namespace.
 * Shared providers are available to all workspaces (read-only).
 */
export function useSharedProviders() {
  const service = useDataService();

  return useQuery({
    queryKey: ["shared-providers", service.name],
    queryFn: async (): Promise<Provider[]> => {
      const providers = await service.getSharedProviders();
      return providers as unknown as Provider[];
    },
    staleTime: 0,
    refetchOnMount: "always",
  });
}

/**
 * Fetches a single shared provider by name.
 */
export function useSharedProvider(name: string) {
  const service = useDataService();

  return useQuery({
    queryKey: ["shared-provider", name, service.name],
    queryFn: async (): Promise<Provider | null> => {
      const provider = await service.getSharedProvider(name) as ServiceProvider | undefined;
      return (provider as unknown as Provider) || null;
    },
    enabled: !!name,
    staleTime: 0,
    refetchOnMount: "always",
  });
}

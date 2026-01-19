"use client";

import { useQuery } from "@tanstack/react-query";
import type { Provider } from "@/types";

/**
 * Fetches shared providers from the system namespace.
 * Shared providers are available to all workspaces (read-only).
 */
export function useSharedProviders() {
  return useQuery({
    queryKey: ["shared-providers"],
    queryFn: async (): Promise<Provider[]> => {
      const response = await fetch("/api/shared/providers");
      if (!response.ok) {
        if (response.status === 401) {
          return []; // Not authenticated, return empty
        }
        throw new Error(`Failed to fetch shared providers: ${response.statusText}`);
      }
      return response.json();
    },
  });
}

/**
 * Fetches a single shared provider by name.
 */
export function useSharedProvider(name: string) {
  return useQuery({
    queryKey: ["shared-provider", name],
    queryFn: async (): Promise<Provider | null> => {
      const response = await fetch(`/api/shared/providers/${encodeURIComponent(name)}`);
      if (!response.ok) {
        if (response.status === 401 || response.status === 404) {
          return null;
        }
        throw new Error(`Failed to fetch shared provider: ${response.statusText}`);
      }
      return response.json();
    },
    enabled: !!name,
  });
}

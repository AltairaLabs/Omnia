"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";
import type { Provider } from "@/types";

export function useProvider(name: string | undefined, namespace: string) {
  const service = useDataService();

  return useQuery({
    queryKey: ["provider", namespace, name, service.name],
    queryFn: async (): Promise<Provider | null> => {
      if (!name) return null;
      const response = await service.getProvider(namespace, name);
      return (response as Provider) || null;
    },
    enabled: !!name,
  });
}

/**
 * Hook for updating a provider's secretRef.
 */
export function useUpdateProviderSecretRef() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({
      namespace,
      name,
      secretRef,
    }: {
      namespace: string;
      name: string;
      secretRef: string | null;
    }) => {
      const response = await fetch(
        `/api/providers/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ secretRef }),
        }
      );

      if (!response.ok) {
        const error = await response.json().catch(() => ({ error: "Failed to update provider" }));
        throw new Error(error.error || "Failed to update provider");
      }

      return response.json();
    },
    onSuccess: (_, variables) => {
      // Invalidate provider queries to refetch
      queryClient.invalidateQueries({ queryKey: ["provider", variables.namespace, variables.name] });
      queryClient.invalidateQueries({ queryKey: ["providers"] });
    },
  });
}

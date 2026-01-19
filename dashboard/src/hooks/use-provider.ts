"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useProvider as useProviderBase } from "./use-providers";

// Re-export useProvider from use-providers.ts
export const useProvider = useProviderBase;

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
      queryClient.invalidateQueries({ queryKey: ["provider", variables.name] });
      queryClient.invalidateQueries({ queryKey: ["providers"] });
    },
  });
}

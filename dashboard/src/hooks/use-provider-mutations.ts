"use client";

import { useState, useCallback } from "react";
import { useWorkspace } from "@/contexts/workspace-context";
import type { Provider, ProviderSpec } from "@/types/generated/provider";

const NO_WORKSPACE_ERROR = "No workspace selected";

interface UseProviderMutationsResult {
  createProvider: (name: string, spec: ProviderSpec) => Promise<Provider>;
  updateProvider: (name: string, spec: ProviderSpec) => Promise<Provider>;
  loading: boolean;
  error: Error | null;
}

/**
 * Hook to provide mutations for Providers (create, update).
 */
export function useProviderMutations(): UseProviderMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const createProvider = useCallback(
    async (name: string, spec: ProviderSpec): Promise<Provider> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(`/api/workspaces/${workspace}/providers`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ metadata: { name }, spec }),
        });

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to create provider");
        }

        return response.json();
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        throw error;
      } finally {
        setLoading(false);
      }
    },
    [workspace]
  );

  const updateProvider = useCallback(
    async (name: string, spec: ProviderSpec): Promise<Provider> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/providers/${name}`,
          {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ spec }),
          }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to update provider");
        }

        return response.json();
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        throw error;
      } finally {
        setLoading(false);
      }
    },
    [workspace]
  );

  return {
    createProvider,
    updateProvider,
    loading,
    error,
  };
}

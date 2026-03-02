"use client";

import { useState, useCallback } from "react";
import { useWorkspace } from "@/contexts/workspace-context";
import type { PromptPack, PromptPackSpec } from "@/types/prompt-pack";

const NO_WORKSPACE_ERROR = "No workspace selected";

interface UsePromptPackMutationsResult {
  createPromptPack: (name: string, spec: PromptPackSpec) => Promise<PromptPack>;
  loading: boolean;
  error: Error | null;
}

/**
 * Hook to provide mutations for PromptPacks (create).
 */
export function usePromptPackMutations(): UsePromptPackMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const createPromptPack = useCallback(
    async (name: string, spec: PromptPackSpec): Promise<PromptPack> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/promptpacks`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ metadata: { name }, spec }),
          }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to create PromptPack");
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
    createPromptPack,
    loading,
    error,
  };
}

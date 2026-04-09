"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import type { Workspace, WorkspaceSpec } from "@/types/workspace";

const WORKSPACE_DETAIL_KEY = "workspace-detail";

async function fetchWorkspaceDetail(name: string): Promise<Workspace> {
  const response = await fetch(`/api/workspaces/${encodeURIComponent(name)}?view=full`);

  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: "Failed to fetch workspace" }));
    throw new Error(error.message || "Failed to fetch workspace");
  }

  return response.json();
}

/**
 * Hook to fetch full detail for a single workspace by name.
 * Pass null to disable fetching.
 */
export function useWorkspaceDetail(name: string | null) {
  return useQuery({
    queryKey: [WORKSPACE_DETAIL_KEY, name],
    queryFn: () => fetchWorkspaceDetail(name!),
    enabled: !!name,
    staleTime: 30000,
  });
}

/**
 * Hook to patch a workspace's spec with optimistic updates.
 * Merges partial spec changes into the cached workspace data immediately,
 * then rolls back on error.
 */
export function useWorkspacePatch(
  name: string,
  options?: { onError?: (err: Error) => void }
) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (updates: Partial<WorkspaceSpec>) => {
      const response = await fetch(`/api/workspaces/${encodeURIComponent(name)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(updates),
      });

      if (!response.ok) {
        const error = await response.json().catch(() => ({ message: "Failed to update workspace" }));
        throw new Error(error.message || "Failed to update workspace");
      }

      return response.json() as Promise<Workspace>;
    },

    onMutate: async (updates: Partial<WorkspaceSpec>) => {
      await queryClient.cancelQueries({ queryKey: [WORKSPACE_DETAIL_KEY, name] });

      const previous = queryClient.getQueryData<Workspace>([WORKSPACE_DETAIL_KEY, name]);

      if (previous) {
        queryClient.setQueryData<Workspace>([WORKSPACE_DETAIL_KEY, name], {
          ...previous,
          spec: { ...previous.spec, ...updates },
        });
      }

      return { previous };
    },

    onError: (err, _updates, context) => {
      if (context?.previous) {
        queryClient.setQueryData([WORKSPACE_DETAIL_KEY, name], context.previous);
      }
      options?.onError?.(err);
    },

    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: [WORKSPACE_DETAIL_KEY, name] });
    },
  });
}

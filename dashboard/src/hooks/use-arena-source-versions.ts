"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import type { ArenaVersion, ArenaVersionsResponse } from "@/types/arena";

const NO_WORKSPACE_ERROR = "No workspace selected";
const EMPTY_VERSIONS: ArenaVersionsResponse = {
  sourceName: "",
  versions: [],
  head: "",
};

interface UseArenaSourceVersionsResult {
  /** List of available versions */
  versions: ArenaVersion[];
  /** Current HEAD version hash */
  headVersion: string | null;
  /** Whether versions are being loaded */
  loading: boolean;
  /** Error if the fetch failed */
  error: Error | null;
  /** Refetch versions */
  refetch: () => void;
}

interface UseArenaSourceVersionMutationsResult {
  /** Switch to a different version */
  switchVersion: (versionHash: string) => Promise<void>;
  /** Whether a version switch is in progress */
  switching: boolean;
  /** Error if the switch failed */
  error: Error | null;
}

function versionsKey(workspace: string | undefined, sourceName: string | undefined) {
  return ["arena-source-versions", workspace, sourceName] as const;
}

async function fetchArenaSourceVersions(
  workspace: string,
  sourceName: string,
): Promise<ArenaVersionsResponse> {
  const response = await fetch(
    `/api/workspaces/${workspace}/arena/sources/${sourceName}/versions`,
  );
  // 404 = source not ready / no versions yet — surface as empty, not an error.
  if (response.status === 404) return EMPTY_VERSIONS;
  if (!response.ok) {
    throw new Error(`Failed to fetch versions: ${response.statusText}`);
  }
  return response.json();
}

/**
 * Hook to fetch versions for an ArenaSource.
 * Returns the list of available versions and the current HEAD version.
 *
 * @param sourceName - Name of the ArenaSource to fetch versions for
 */
export function useArenaSourceVersions(
  sourceName: string | undefined,
): UseArenaSourceVersionsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const query = useQuery({
    queryKey: versionsKey(workspace, sourceName),
    queryFn: () => fetchArenaSourceVersions(workspace!, sourceName!),
    enabled: !!workspace && !!sourceName,
  });

  const data = query.data ?? EMPTY_VERSIONS;

  return {
    versions: data.versions,
    headVersion: data.head || null,
    loading: query.isLoading,
    error: (query.error as Error | null) ?? null,
    refetch: async () => { await query.refetch(); },
  };
}

/**
 * Hook to provide version switch mutations for an ArenaSource.
 *
 * @param sourceName - Name of the ArenaSource to switch versions for
 * @param onSuccess - Callback to execute after successful version switch
 */
export function useArenaSourceVersionMutations(
  sourceName: string | undefined,
  onSuccess?: () => void,
): UseArenaSourceVersionMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const queryClient = useQueryClient();

  const mutation = useMutation({
    mutationFn: async (versionHash: string): Promise<void> => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      if (!sourceName) throw new Error("No source name provided");
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/sources/${sourceName}/versions`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ version: versionHash }),
        },
      );
      if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        throw new Error(
          data.error || `Failed to switch version: ${response.statusText}`,
        );
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: versionsKey(workspace, sourceName),
      });
      onSuccess?.();
    },
  });

  return {
    switchVersion: mutation.mutateAsync,
    switching: mutation.isPending,
    error: (mutation.error as Error | null) ?? null,
  };
}

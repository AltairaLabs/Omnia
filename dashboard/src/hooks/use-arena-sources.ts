"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import type { ArenaSource } from "@/types/arena";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";

const NO_WORKSPACE_ERROR = "No workspace selected";

interface UseArenaSourcesResult {
  sources: ArenaSource[];
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

interface UseArenaSourceResult {
  source: ArenaSource | null;
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

interface UseArenaSourceMutationsResult {
  createSource: (name: string, spec: ArenaSource["spec"]) => Promise<ArenaSource>;
  updateSource: (name: string, spec: ArenaSource["spec"]) => Promise<ArenaSource>;
  deleteSource: (name: string) => Promise<void>;
  syncSource: (name: string) => Promise<void>;
  loading: boolean;
  error: Error | null;
}

async function fetchSources(workspace: string): Promise<ArenaSource[]> {
  const response = await fetch(`/api/workspaces/${workspace}/arena/sources`);
  if (!response.ok) {
    throw new Error(`Failed to fetch sources: ${response.statusText}`);
  }
  return response.json();
}

async function fetchSource(workspace: string, name: string): Promise<ArenaSource> {
  const response = await fetch(`/api/workspaces/${workspace}/arena/sources/${name}`);
  if (!response.ok) {
    if (response.status === 404) {
      throw new Error("Source not found");
    }
    throw new Error(`Failed to fetch source: ${response.statusText}`);
  }
  return response.json();
}

/**
 * Hook to fetch Arena sources for the current workspace.
 */
export function useArenaSources(): UseArenaSourcesResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["arena-sources", workspace],
    queryFn: () => fetchSources(workspace!),
    enabled: !!workspace,
    staleTime: DEFAULT_STALE_TIME,
  });

  return {
    sources: data ?? [],
    loading: isLoading,
    error: error as Error | null,
    refetch,
  };
}

/**
 * Hook to fetch a single Arena source.
 */
export function useArenaSource(name: string | undefined): UseArenaSourceResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["arena-source", workspace, name],
    queryFn: () => fetchSource(workspace!, name!),
    enabled: !!workspace && !!name,
    staleTime: DEFAULT_STALE_TIME,
  });

  return {
    source: data ?? null,
    loading: isLoading,
    error: error as Error | null,
    refetch,
  };
}

async function handleMutationResponse(response: Response, fallbackMsg: string): Promise<Response> {
  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(errorText || fallbackMsg);
  }
  return response;
}

/**
 * Hook to provide mutations for Arena sources (create, update, delete, sync).
 */
export function useArenaSourceMutations(): UseArenaSourceMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const queryClient = useQueryClient();

  const invalidateSources = () => {
    queryClient.invalidateQueries({ queryKey: ["arena-sources", workspace] });
  };

  const createMutation = useMutation({
    mutationFn: async ({ name, spec }: { name: string; spec: ArenaSource["spec"] }) => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(`/api/workspaces/${workspace}/arena/sources`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ metadata: { name }, spec }),
      });
      await handleMutationResponse(response, "Failed to create source");
      return response.json() as Promise<ArenaSource>;
    },
    onSuccess: invalidateSources,
  });

  const updateMutation = useMutation({
    mutationFn: async ({ name, spec }: { name: string; spec: ArenaSource["spec"] }) => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(`/api/workspaces/${workspace}/arena/sources/${name}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ spec }),
      });
      await handleMutationResponse(response, "Failed to update source");
      return response.json() as Promise<ArenaSource>;
    },
    onSuccess: invalidateSources,
  });

  const deleteMutation = useMutation({
    mutationFn: async (name: string) => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(`/api/workspaces/${workspace}/arena/sources/${name}`, {
        method: "DELETE",
      });
      await handleMutationResponse(response, "Failed to delete source");
    },
    onSuccess: invalidateSources,
  });

  const syncMutation = useMutation({
    mutationFn: async (name: string) => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(`/api/workspaces/${workspace}/arena/sources/${name}/sync`, {
        method: "POST",
      });
      await handleMutationResponse(response, "Failed to sync source");
    },
    onSuccess: invalidateSources,
  });

  const isPending = createMutation.isPending || updateMutation.isPending
    || deleteMutation.isPending || syncMutation.isPending;

  const activeError = createMutation.error ?? updateMutation.error
    ?? deleteMutation.error ?? syncMutation.error;

  return {
    createSource: (name: string, spec: ArenaSource["spec"]) =>
      createMutation.mutateAsync({ name, spec }),
    updateSource: (name: string, spec: ArenaSource["spec"]) =>
      updateMutation.mutateAsync({ name, spec }),
    deleteSource: (name: string) => deleteMutation.mutateAsync(name),
    syncSource: (name: string) => syncMutation.mutateAsync(name),
    loading: isPending,
    error: (activeError as Error) ?? null,
  };
}

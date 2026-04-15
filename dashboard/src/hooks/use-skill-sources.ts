"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import type { SkillSource, SkillSourceSpec } from "@/types/skill-source";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";

const NO_WORKSPACE_ERROR = "No workspace selected";

async function handleMutationError(response: Response, fallback: string): Promise<never> {
  try {
    const body = await response.json();
    throw new Error(body?.error || body?.message || fallback);
  } catch (err) {
    if (err instanceof Error) throw err;
    throw new Error(fallback);
  }
}

async function fetchSkillSources(workspace: string): Promise<SkillSource[]> {
  const response = await fetch(`/api/workspaces/${workspace}/skills`);
  if (!response.ok) {
    throw new Error(`Failed to fetch skill sources: ${response.statusText}`);
  }
  return response.json();
}

async function fetchSkillSource(
  workspace: string,
  name: string
): Promise<SkillSource> {
  const response = await fetch(`/api/workspaces/${workspace}/skills/${name}`);
  if (!response.ok) {
    if (response.status === 404) {
      throw new Error("Skill source not found");
    }
    throw new Error(`Failed to fetch skill source: ${response.statusText}`);
  }
  return response.json();
}

/**
 * Fetch SkillSource CRDs for the current workspace. Issue #829.
 */
export function useSkillSources() {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["skill-sources", workspace],
    queryFn: () => (workspace ? fetchSkillSources(workspace) : []),
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
 * Mutations for SkillSource CRDs in the current workspace.
 */
export function useSkillSourceMutations() {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const queryClient = useQueryClient();

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["skill-sources", workspace] });
  };

  const create = useMutation({
    mutationFn: async ({
      name,
      spec,
    }: {
      name: string;
      spec: SkillSourceSpec;
    }): Promise<SkillSource> => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(`/api/workspaces/${workspace}/skills`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ metadata: { name }, spec }),
      });
      if (!response.ok) {
        await handleMutationError(response, "Failed to create skill source");
      }
      return response.json();
    },
    onSuccess: invalidate,
  });

  const update = useMutation({
    mutationFn: async ({
      name,
      spec,
    }: {
      name: string;
      spec: SkillSourceSpec;
    }): Promise<SkillSource> => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(
        `/api/workspaces/${workspace}/skills/${name}`,
        {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ metadata: { name }, spec }),
        }
      );
      if (!response.ok) {
        await handleMutationError(response, "Failed to update skill source");
      }
      return response.json();
    },
    onSuccess: invalidate,
  });

  const remove = useMutation({
    mutationFn: async (name: string): Promise<void> => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(
        `/api/workspaces/${workspace}/skills/${name}`,
        { method: "DELETE" }
      );
      if (!response.ok) {
        await handleMutationError(response, "Failed to delete skill source");
      }
    },
    onSuccess: invalidate,
  });

  return {
    createSource: (name: string, spec: SkillSourceSpec) =>
      create.mutateAsync({ name, spec }),
    updateSource: (name: string, spec: SkillSourceSpec) =>
      update.mutateAsync({ name, spec }),
    deleteSource: (name: string) => remove.mutateAsync(name),
    loading: create.isPending || update.isPending || remove.isPending,
    error: (create.error || update.error || remove.error) as Error | null,
  };
}

/**
 * Fetch a single SkillSource by name in the current workspace.
 */
export function useSkillSource(name: string | undefined) {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["skill-source", workspace, name],
    queryFn: () =>
      workspace && name ? fetchSkillSource(workspace, name) : null,
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

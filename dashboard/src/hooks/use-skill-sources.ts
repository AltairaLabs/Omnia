"use client";

import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import type { SkillSource } from "@/types/skill-source";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";

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

"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import type { ArenaSource, ArenaJob, ArenaJobType, ScenarioFilter } from "@/types/arena";

const NO_WORKSPACE_ERROR = "No workspace selected";

// =============================================================================
// Types
// =============================================================================

export interface ProjectJobsResponse {
  jobs: ArenaJob[];
  source?: ArenaSource;
  deployed: boolean;
}

export interface QuickRunRequest {
  type: ArenaJobType;
  name?: string;
  scenarios?: ScenarioFilter;
  verbose?: boolean;
}

export interface QuickRunResponse {
  job: ArenaJob;
  source: ArenaSource;
}

export interface ProjectJobsFilter {
  type?: ArenaJobType;
  status?: string;
  limit?: number;
}

const EMPTY_RESPONSE: ProjectJobsResponse = { jobs: [], deployed: false };

function jobsKey(
  workspace: string | undefined,
  projectId: string | undefined,
  filter: ProjectJobsFilter | undefined,
) {
  return [
    "project-jobs",
    workspace,
    projectId,
    filter?.type ?? null,
    filter?.status ?? null,
    filter?.limit ?? null,
  ] as const;
}

async function fetchProjectJobs(
  workspace: string,
  projectId: string,
  filter: ProjectJobsFilter | undefined,
): Promise<ProjectJobsResponse> {
  const params = new URLSearchParams();
  if (filter?.type) params.set("type", filter.type);
  if (filter?.status) params.set("status", filter.status);
  if (filter?.limit) params.set("limit", String(filter.limit));
  const queryString = params.toString();
  const url = `/api/workspaces/${workspace}/arena/projects/${projectId}/jobs${
    queryString ? `?${queryString}` : ""
  }`;
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`Failed to list jobs: ${response.statusText}`);
  }
  return response.json();
}

// =============================================================================
// Project Jobs List Hook
// =============================================================================

interface UseProjectJobsResult {
  jobs: ArenaJob[];
  source?: ArenaSource;
  deployed: boolean;
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

/**
 * Hook to list jobs for a project.
 */
export function useProjectJobs(
  projectId: string | undefined,
  filter?: ProjectJobsFilter,
): UseProjectJobsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const query = useQuery({
    queryKey: jobsKey(workspace, projectId, filter),
    queryFn: () => fetchProjectJobs(workspace!, projectId!, filter),
    enabled: !!workspace && !!projectId,
  });

  const data = query.data ?? EMPTY_RESPONSE;

  return {
    jobs: data.jobs,
    source: data.source,
    deployed: data.deployed,
    loading: query.isLoading,
    error: (query.error as Error | null) ?? null,
    refetch: async () => { await query.refetch(); },
  };
}

// =============================================================================
// Project Run Mutations Hook
// =============================================================================

interface UseProjectRunMutationsResult {
  run: (projectId: string, options: QuickRunRequest) => Promise<QuickRunResponse>;
  running: boolean;
  error: Error | null;
}

/**
 * Hook to run a project as an ArenaJob.
 */
export function useProjectRunMutations(): UseProjectRunMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const queryClient = useQueryClient();

  const mutation = useMutation({
    mutationFn: async (
      args: { projectId: string; options: QuickRunRequest },
    ): Promise<QuickRunResponse> => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/projects/${args.projectId}/run`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(args.options),
        },
      );
      if (!response.ok) {
        const errorData = await response.json().catch(() => null);
        const message =
          errorData?.message || errorData?.error || "Failed to run project";
        throw new Error(message);
      }
      return response.json();
    },
    onSuccess: (_data, variables) => {
      // Invalidate any jobs query for this project, regardless of filter.
      queryClient.invalidateQueries({
        queryKey: ["project-jobs", workspace, variables.projectId],
      });
    },
  });

  return {
    run: (projectId, options) => mutation.mutateAsync({ projectId, options }),
    running: mutation.isPending,
    error: (mutation.error as Error | null) ?? null,
  };
}

// =============================================================================
// Combined Hook
// =============================================================================

interface UseProjectJobsWithRunResult {
  jobs: ArenaJob[];
  source?: ArenaSource;
  deployed: boolean;
  loading: boolean;
  running: boolean;
  error: Error | null;
  run: (options: QuickRunRequest) => Promise<QuickRunResponse>;
  refetch: () => void;
}

/**
 * Combined hook for project jobs and running.
 */
export function useProjectJobsWithRun(
  projectId: string | undefined,
  filter?: ProjectJobsFilter,
): UseProjectJobsWithRunResult {
  const { jobs, source, deployed, loading, error: listError, refetch } = useProjectJobs(
    projectId,
    filter,
  );
  const { run: runMutation, running, error: runError } = useProjectRunMutations();

  const run = async (options: QuickRunRequest): Promise<QuickRunResponse> => {
    if (!projectId) throw new Error("No project selected");
    return runMutation(projectId, options);
  };

  return {
    jobs,
    source,
    deployed,
    loading,
    running,
    error: listError || runError,
    run,
    refetch,
  };
}

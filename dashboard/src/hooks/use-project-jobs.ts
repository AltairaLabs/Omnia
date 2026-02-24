"use client";

import { useState, useEffect, useCallback } from "react";
import { useWorkspace } from "@/contexts/workspace-context";
import type { ArenaSource, ArenaJob, ArenaJobType, ScenarioFilter, ExecutionConfig } from "@/types/arena";

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
  execution?: ExecutionConfig;
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
  filter?: ProjectJobsFilter
): UseProjectJobsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [jobs, setJobs] = useState<ArenaJob[]>([]);
  const [source, setSource] = useState<ArenaSource | undefined>(undefined);
  const [deployed, setDeployed] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace || !projectId) {
      setJobs([]);
      setSource(undefined);
      setDeployed(false);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
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

      const data: ProjectJobsResponse = await response.json();
      setJobs(data.jobs);
      setSource(data.source);
      setDeployed(data.deployed);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setJobs([]);
      setSource(undefined);
      setDeployed(false);
    } finally {
      setLoading(false);
    }
  }, [workspace, projectId, filter?.type, filter?.status, filter?.limit]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    jobs,
    source,
    deployed,
    loading,
    error,
    refetch: fetchData,
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
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const run = useCallback(
    async (projectId: string, options: QuickRunRequest): Promise<QuickRunResponse> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setRunning(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/projects/${projectId}/run`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(options),
          }
        );

        if (!response.ok) {
          const errorData = await response.json().catch(() => null);
          const message =
            errorData?.message || errorData?.error || "Failed to run project";
          throw new Error(message);
        }

        return response.json();
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        throw error;
      } finally {
        setRunning(false);
      }
    },
    [workspace]
  );

  return {
    run,
    running,
    error,
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
  filter?: ProjectJobsFilter
): UseProjectJobsWithRunResult {
  const { jobs, source, deployed, loading, error: listError, refetch } = useProjectJobs(
    projectId,
    filter
  );
  const { run: runMutation, running, error: runError } = useProjectRunMutations();

  const run = useCallback(
    async (options: QuickRunRequest): Promise<QuickRunResponse> => {
      if (!projectId) {
        throw new Error("No project selected");
      }

      const result = await runMutation(projectId, options);
      // Refresh jobs list after successful run
      refetch();
      return result;
    },
    [projectId, runMutation, refetch]
  );

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

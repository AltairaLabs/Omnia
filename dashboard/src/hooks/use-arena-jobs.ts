"use client";

import { useState, useEffect, useCallback } from "react";
import { useWorkspace } from "@/contexts/workspace-context";
import type { ArenaJob, ArenaJobPhase, ArenaJobType } from "@/types/arena";

const NO_WORKSPACE_ERROR = "No workspace selected";

interface UseArenaJobsOptions {
  configRef?: string;
  type?: ArenaJobType;
  phase?: ArenaJobPhase;
}

interface UseArenaJobsResult {
  jobs: ArenaJob[];
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

interface UseArenaJobResult {
  job: ArenaJob | null;
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

interface UseArenaJobMutationsResult {
  createJob: (name: string, spec: ArenaJob["spec"]) => Promise<ArenaJob>;
  cancelJob: (name: string) => Promise<ArenaJob>;
  deleteJob: (name: string) => Promise<void>;
  loading: boolean;
  error: Error | null;
}

/**
 * Hook to fetch Arena jobs for the current workspace.
 * Supports optional filtering by configRef, type, or phase.
 */
export function useArenaJobs(options?: UseArenaJobsOptions): UseArenaJobsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [jobs, setJobs] = useState<ArenaJob[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace) {
      setJobs([]);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const params = new URLSearchParams();
      if (options?.configRef) params.set("configRef", options.configRef);
      if (options?.type) params.set("type", options.type);
      if (options?.phase) params.set("phase", options.phase);

      const queryString = params.toString();
      const suffix = queryString ? `?${queryString}` : "";
      const url = `/api/workspaces/${workspace}/arena/jobs${suffix}`;

      const response = await fetch(url);
      if (!response.ok) {
        throw new Error(`Failed to fetch jobs: ${response.statusText}`);
      }
      const data = await response.json();
      setJobs(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setJobs([]);
    } finally {
      setLoading(false);
    }
  }, [workspace, options?.configRef, options?.type, options?.phase]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    jobs,
    loading,
    error,
    refetch: fetchData,
  };
}

/**
 * Hook to fetch a single Arena job by name.
 */
export function useArenaJob(name: string | undefined): UseArenaJobResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [job, setJob] = useState<ArenaJob | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace || !name) {
      setJob(null);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(`/api/workspaces/${workspace}/arena/jobs/${name}`);

      if (!response.ok) {
        if (response.status === 404) {
          throw new Error("Job not found");
        }
        throw new Error(`Failed to fetch job: ${response.statusText}`);
      }

      const jobData = await response.json();
      setJob(jobData);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setJob(null);
    } finally {
      setLoading(false);
    }
  }, [workspace, name]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    job,
    loading,
    error,
    refetch: fetchData,
  };
}

/**
 * Hook to provide mutations for Arena jobs (create, cancel, delete).
 */
export function useArenaJobMutations(): UseArenaJobMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const createJob = useCallback(
    async (name: string, spec: ArenaJob["spec"]): Promise<ArenaJob> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(`/api/workspaces/${workspace}/arena/jobs`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ metadata: { name }, spec }),
        });

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to create job");
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

  const cancelJob = useCallback(
    async (name: string): Promise<ArenaJob> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/jobs/${name}/cancel`,
          { method: "POST" }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to cancel job");
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

  const deleteJob = useCallback(
    async (name: string): Promise<void> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/jobs/${name}`,
          { method: "DELETE" }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to delete job");
        }
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
    createJob,
    cancelJob,
    deleteJob,
    loading,
    error,
  };
}

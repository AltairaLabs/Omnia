"use client";

/**
 * Arena Jobs hooks for fetching and mutating Arena jobs.
 *
 * DEMO MODE SUPPORT:
 * These hooks use the DataService abstraction to support both demo mode (mock data)
 * and live mode (real K8s API). When adding new functionality:
 * 1. Add the method to DataService interface in src/lib/data/types.ts
 * 2. Implement in MockDataService (src/lib/data/mock-service.ts) for demo mode
 * 3. Implement in LiveDataService (src/lib/data/live-service.ts) for production
 * 4. Use useDataService() in hooks to get the appropriate implementation
 *
 * This ensures the UI works in demo mode without requiring K8s access.
 */

import { useState, useEffect, useCallback } from "react";
import { useWorkspace } from "@/contexts/workspace-context";
import { useDataService } from "@/lib/data/provider";
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
 *
 * Uses DataService for demo/live mode support.
 */
export function useArenaJobs(options?: UseArenaJobsOptions): UseArenaJobsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const service = useDataService();
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
      const data = await service.getArenaJobs(workspace, {
        configRef: options?.configRef,
        type: options?.type,
        phase: options?.phase,
      });
      setJobs(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setJobs([]);
    } finally {
      setLoading(false);
    }
  }, [workspace, service, options?.configRef, options?.type, options?.phase]);

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
 *
 * Uses DataService for demo/live mode support.
 */
export function useArenaJob(name: string | undefined): UseArenaJobResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const service = useDataService();
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
      const jobData = await service.getArenaJob(workspace, name);
      if (!jobData) {
        throw new Error("Job not found");
      }
      setJob(jobData);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setJob(null);
    } finally {
      setLoading(false);
    }
  }, [workspace, name, service]);

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
 *
 * Uses DataService for demo/live mode support.
 */
export function useArenaJobMutations(): UseArenaJobMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const service = useDataService();
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
        return await service.createArenaJob(workspace, name, spec);
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        throw error;
      } finally {
        setLoading(false);
      }
    },
    [workspace, service]
  );

  const cancelJob = useCallback(
    async (name: string): Promise<ArenaJob> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        await service.cancelArenaJob(workspace, name);
        // Fetch and return the updated job
        const job = await service.getArenaJob(workspace, name);
        if (!job) {
          throw new Error("Job not found after cancel");
        }
        return job;
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        throw error;
      } finally {
        setLoading(false);
      }
    },
    [workspace, service]
  );

  const deleteJob = useCallback(
    async (name: string): Promise<void> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        await service.deleteArenaJob(workspace, name);
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        throw error;
      } finally {
        setLoading(false);
      }
    },
    [workspace, service]
  );

  return {
    createJob,
    cancelJob,
    deleteJob,
    loading,
    error,
  };
}

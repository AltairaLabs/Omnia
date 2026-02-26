"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import type { ArenaJob, ArenaJobPhase, ArenaJobType } from "@/types/arena";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";

const NO_WORKSPACE_ERROR = "No workspace selected";

interface UseArenaJobsOptions {
  sourceRef?: string;
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

function buildJobsUrl(workspace: string, options?: UseArenaJobsOptions): string {
  const params = new URLSearchParams();
  if (options?.sourceRef) params.set("sourceRef", options.sourceRef);
  if (options?.type) params.set("type", options.type);
  if (options?.phase) params.set("phase", options.phase);

  const queryString = params.toString();
  const suffix = queryString ? `?${queryString}` : "";
  return `/api/workspaces/${workspace}/arena/jobs${suffix}`;
}

async function fetchJobs(workspace: string, options?: UseArenaJobsOptions): Promise<ArenaJob[]> {
  const url = buildJobsUrl(workspace, options);
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`Failed to fetch jobs: ${response.statusText}`);
  }
  return response.json();
}

async function fetchJob(workspace: string, name: string): Promise<ArenaJob> {
  const response = await fetch(`/api/workspaces/${workspace}/arena/jobs/${name}`);
  if (!response.ok) {
    if (response.status === 404) {
      throw new Error("Job not found");
    }
    throw new Error(`Failed to fetch job: ${response.statusText}`);
  }
  return response.json();
}

/**
 * Hook to fetch Arena jobs for the current workspace.
 * Supports optional filtering by sourceRef, type, or phase.
 */
export function useArenaJobs(options?: UseArenaJobsOptions): UseArenaJobsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["arena-jobs", workspace, options?.sourceRef, options?.type, options?.phase],
    queryFn: () => fetchJobs(workspace!, options),
    enabled: !!workspace,
    staleTime: DEFAULT_STALE_TIME,
  });

  return {
    jobs: data ?? [],
    loading: isLoading,
    error: error as Error | null,
    refetch,
  };
}

/**
 * Hook to fetch a single Arena job by name.
 */
export function useArenaJob(name: string | undefined): UseArenaJobResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["arena-job", workspace, name],
    queryFn: () => fetchJob(workspace!, name!),
    enabled: !!workspace && !!name,
    staleTime: DEFAULT_STALE_TIME,
  });

  return {
    job: data ?? null,
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
 * Hook to provide mutations for Arena jobs (create, cancel, delete).
 */
export function useArenaJobMutations(): UseArenaJobMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const queryClient = useQueryClient();

  const invalidateJobs = () => {
    queryClient.invalidateQueries({ queryKey: ["arena-jobs", workspace] });
  };

  const createMutation = useMutation({
    mutationFn: async ({ name, spec }: { name: string; spec: ArenaJob["spec"] }) => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(`/api/workspaces/${workspace}/arena/jobs`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ metadata: { name }, spec }),
      });
      await handleMutationResponse(response, "Failed to create job");
      return response.json() as Promise<ArenaJob>;
    },
    onSuccess: invalidateJobs,
  });

  const cancelMutation = useMutation({
    mutationFn: async (name: string) => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(`/api/workspaces/${workspace}/arena/jobs/${name}/cancel`, {
        method: "POST",
      });
      await handleMutationResponse(response, "Failed to cancel job");
      return response.json() as Promise<ArenaJob>;
    },
    onSuccess: invalidateJobs,
  });

  const deleteMutation = useMutation({
    mutationFn: async (name: string) => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(`/api/workspaces/${workspace}/arena/jobs/${name}`, {
        method: "DELETE",
      });
      await handleMutationResponse(response, "Failed to delete job");
    },
    onSuccess: invalidateJobs,
  });

  const isPending = createMutation.isPending || cancelMutation.isPending
    || deleteMutation.isPending;

  const activeError = createMutation.error ?? cancelMutation.error
    ?? deleteMutation.error;

  return {
    createJob: (name: string, spec: ArenaJob["spec"]) =>
      createMutation.mutateAsync({ name, spec }),
    cancelJob: (name: string) => cancelMutation.mutateAsync(name),
    deleteJob: (name: string) => deleteMutation.mutateAsync(name),
    loading: isPending,
    error: (activeError as Error) ?? null,
  };
}

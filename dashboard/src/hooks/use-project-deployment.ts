"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import type { ArenaSource } from "@/types/arena";

const NO_WORKSPACE_ERROR = "No workspace selected";

// =============================================================================
// Types
// =============================================================================

export interface ConfigMapInfo {
  name: string;
  namespace: string;
  fileCount: number;
  createdAt?: string;
  updatedAt?: string;
}

export interface DeploymentStatus {
  deployed: boolean;
  source?: ArenaSource;
  configMap?: ConfigMapInfo;
  lastDeployedAt?: string;
}

export interface DeployRequest {
  name?: string;
  syncInterval?: string;
}

export interface DeployResponse {
  source: ArenaSource;
  configMap: { name: string; namespace: string };
  isNew: boolean;
}

function deploymentKey(workspace: string | undefined, projectId: string | undefined) {
  return ["project-deployment-status", workspace, projectId] as const;
}

async function fetchDeploymentStatus(
  workspace: string,
  projectId: string,
): Promise<DeploymentStatus> {
  const response = await fetch(
    `/api/workspaces/${workspace}/arena/projects/${projectId}/deployment`,
  );
  if (!response.ok) {
    throw new Error(`Failed to get deployment status: ${response.statusText}`);
  }
  return response.json();
}

// =============================================================================
// Deployment Status Hook
// =============================================================================

interface UseProjectDeploymentStatusResult {
  status: DeploymentStatus | null;
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

/**
 * Hook to get the deployment status of a project.
 */
export function useProjectDeploymentStatus(
  projectId: string | undefined,
): UseProjectDeploymentStatusResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const query = useQuery({
    queryKey: deploymentKey(workspace, projectId),
    queryFn: () => fetchDeploymentStatus(workspace!, projectId!),
    enabled: !!workspace && !!projectId,
  });

  return {
    status: query.data ?? null,
    loading: query.isLoading,
    error: (query.error as Error | null) ?? null,
    refetch: async () => { await query.refetch(); },
  };
}

// =============================================================================
// Deployment Mutations Hook
// =============================================================================

interface UseProjectDeploymentMutationsResult {
  deploy: (projectId: string, options?: DeployRequest) => Promise<DeployResponse>;
  deploying: boolean;
  error: Error | null;
}

/**
 * Hook to deploy a project as an ArenaSource.
 */
export function useProjectDeploymentMutations(): UseProjectDeploymentMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const queryClient = useQueryClient();

  const mutation = useMutation({
    mutationFn: async (
      args: { projectId: string; options?: DeployRequest },
    ): Promise<DeployResponse> => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/projects/${args.projectId}/deploy`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(args.options || {}),
        },
      );
      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText || "Failed to deploy project");
      }
      return response.json();
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: deploymentKey(workspace, variables.projectId),
      });
    },
  });

  return {
    deploy: (projectId, options) => mutation.mutateAsync({ projectId, options }),
    deploying: mutation.isPending,
    error: (mutation.error as Error | null) ?? null,
  };
}

// =============================================================================
// Combined Hook
// =============================================================================

interface UseProjectDeploymentResult {
  status: DeploymentStatus | null;
  loading: boolean;
  deploying: boolean;
  error: Error | null;
  deploy: (options?: DeployRequest) => Promise<DeployResponse>;
  refetch: () => void;
}

/**
 * Combined hook for project deployment status and mutations.
 */
export function useProjectDeployment(
  projectId: string | undefined,
): UseProjectDeploymentResult {
  const { status, loading, error: statusError, refetch } = useProjectDeploymentStatus(projectId);
  const { deploy: deployMutation, deploying, error: deployError } = useProjectDeploymentMutations();

  const deploy = async (options?: DeployRequest): Promise<DeployResponse> => {
    if (!projectId) throw new Error("No project selected");
    return deployMutation(projectId, options);
  };

  return {
    status,
    loading,
    deploying,
    error: statusError || deployError,
    deploy,
    refetch,
  };
}

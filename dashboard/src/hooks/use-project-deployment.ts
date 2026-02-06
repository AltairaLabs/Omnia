"use client";

import { useState, useEffect, useCallback } from "react";
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
  projectId: string | undefined
): UseProjectDeploymentStatusResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [status, setStatus] = useState<DeploymentStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace || !projectId) {
      setStatus(null);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/projects/${projectId}/deployment`
      );

      if (!response.ok) {
        throw new Error(`Failed to get deployment status: ${response.statusText}`);
      }

      const data: DeploymentStatus = await response.json();
      setStatus(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setStatus(null);
    } finally {
      setLoading(false);
    }
  }, [workspace, projectId]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    status,
    loading,
    error,
    refetch: fetchData,
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
  const [deploying, setDeploying] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const deploy = useCallback(
    async (projectId: string, options?: DeployRequest): Promise<DeployResponse> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setDeploying(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/projects/${projectId}/deploy`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(options || {}),
          }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to deploy project");
        }

        return response.json();
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        throw error;
      } finally {
        setDeploying(false);
      }
    },
    [workspace]
  );

  return {
    deploy,
    deploying,
    error,
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
  projectId: string | undefined
): UseProjectDeploymentResult {
  const { status, loading, error: statusError, refetch } = useProjectDeploymentStatus(projectId);
  const { deploy: deployMutation, deploying, error: deployError } = useProjectDeploymentMutations();

  const deploy = useCallback(
    async (options?: DeployRequest): Promise<DeployResponse> => {
      if (!projectId) {
        throw new Error("No project selected");
      }

      const result = await deployMutation(projectId, options);
      // Refresh status after successful deploy
      refetch();
      return result;
    },
    [projectId, deployMutation, refetch]
  );

  return {
    status,
    loading,
    deploying,
    error: statusError || deployError,
    deploy,
    refetch,
  };
}

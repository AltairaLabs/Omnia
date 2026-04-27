"use client";

import { useCallback, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import type {
  ArenaProject,
  ArenaProjectWithTree,
  ProjectCreateRequest,
  FileTreeNode,
  FileContentResponse,
  FileUpdateResponse,
  FileCreateResponse,
} from "@/types/arena-project";

const NO_WORKSPACE_ERROR = "No workspace selected";

function projectsKey(workspace: string | undefined) {
  return ["arena-projects", workspace] as const;
}

function projectKey(workspace: string | undefined, projectId: string | undefined) {
  return ["arena-project", workspace, projectId] as const;
}

// =============================================================================
// Project List Hook
// =============================================================================

interface UseArenaProjectsResult {
  projects: ArenaProject[];
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

async function fetchArenaProjects(workspace: string): Promise<ArenaProject[]> {
  try {
    const response = await fetch(`/api/workspaces/${workspace}/arena/projects`);
    if (!response.ok) {
      throw new Error(`Failed to fetch projects: ${response.statusText}`);
    }
    const data = await response.json();
    return data.projects || [];
  } catch (err) {
    // Normalize non-Error throws (e.g. raw string rejections) to Error so
    // consumers can rely on `.message`.
    throw err instanceof Error ? err : new Error(String(err));
  }
}

/**
 * Hook to fetch Arena projects for the current workspace.
 */
export function useArenaProjects(): UseArenaProjectsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const query = useQuery({
    queryKey: projectsKey(workspace),
    queryFn: () => fetchArenaProjects(workspace!),
    enabled: !!workspace,
  });

  return {
    projects: query.data ?? [],
    loading: query.isLoading,
    error: (query.error as Error | null) ?? null,
    refetch: async () => { await query.refetch(); },
  };
}

// =============================================================================
// Single Project Hook
// =============================================================================

interface UseArenaProjectResult {
  project: ArenaProjectWithTree | null;
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

async function fetchArenaProject(
  workspace: string,
  projectId: string,
): Promise<ArenaProjectWithTree> {
  const response = await fetch(
    `/api/workspaces/${workspace}/arena/projects/${projectId}`,
  );
  if (!response.ok) {
    if (response.status === 404) throw new Error("Project not found");
    throw new Error(`Failed to fetch project: ${response.statusText}`);
  }
  return response.json();
}

/**
 * Hook to fetch a single Arena project with its file tree.
 */
export function useArenaProject(projectId: string | undefined): UseArenaProjectResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const query = useQuery({
    queryKey: projectKey(workspace, projectId),
    queryFn: () => fetchArenaProject(workspace!, projectId!),
    enabled: !!workspace && !!projectId,
  });

  return {
    project: query.data ?? null,
    loading: query.isLoading,
    error: (query.error as Error | null) ?? null,
    refetch: async () => { await query.refetch(); },
  };
}

// =============================================================================
// Project Mutations Hook
// =============================================================================

interface UseArenaProjectMutationsResult {
  createProject: (data: ProjectCreateRequest) => Promise<ArenaProject>;
  deleteProject: (projectId: string) => Promise<void>;
  loading: boolean;
  error: Error | null;
}

/**
 * Hook to provide mutations for Arena projects (create, delete).
 */
export function useArenaProjectMutations(): UseArenaProjectMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const queryClient = useQueryClient();

  const createMutation = useMutation({
    mutationFn: async (data: ProjectCreateRequest): Promise<ArenaProject> => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(`/api/workspaces/${workspace}/arena/projects`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });
      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText || "Failed to create project");
      }
      return response.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectsKey(workspace) });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (projectId: string): Promise<void> => {
      if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/projects/${projectId}`,
        { method: "DELETE" },
      );
      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText || "Failed to delete project");
      }
    },
    onSuccess: (_data, projectId) => {
      queryClient.invalidateQueries({ queryKey: projectsKey(workspace) });
      queryClient.invalidateQueries({ queryKey: projectKey(workspace, projectId) });
    },
  });

  return {
    createProject: createMutation.mutateAsync,
    deleteProject: deleteMutation.mutateAsync,
    loading: createMutation.isPending || deleteMutation.isPending,
    error:
      (createMutation.error as Error | null) ??
      (deleteMutation.error as Error | null) ??
      null,
  };
}

// =============================================================================
// File Operations Hook
// =============================================================================

interface UseArenaProjectFilesResult {
  getFileContent: (projectId: string, filePath: string) => Promise<FileContentResponse>;
  updateFileContent: (projectId: string, filePath: string, content: string) => Promise<FileUpdateResponse>;
  createFile: (projectId: string, parentPath: string | null, name: string, isDirectory: boolean, content?: string) => Promise<FileCreateResponse>;
  deleteFile: (projectId: string, filePath: string) => Promise<void>;
  refreshFileTree: (projectId: string) => Promise<FileTreeNode[]>;
  loading: boolean;
  error: Error | null;
}

/**
 * Hook to provide file operations for Arena projects.
 *
 * File ops stay imperative (called from event handlers, not on mount).
 * Successful writes invalidate the parent project query so the file tree
 * refreshes for any consumer.
 */
export function useArenaProjectFiles(): UseArenaProjectFilesResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const queryClient = useQueryClient();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const invalidateProject = useCallback(
    (projectId: string) => {
      queryClient.invalidateQueries({
        queryKey: projectKey(workspace, projectId),
      });
    },
    [queryClient, workspace],
  );

  const wrap = useCallback(async function <T>(fn: () => Promise<T>): Promise<T> {
    setLoading(true);
    setError(null);
    try {
      return await fn();
    } catch (err) {
      const e = err instanceof Error ? err : new Error(String(err));
      setError(e);
      throw e;
    } finally {
      setLoading(false);
    }
  }, []);

  const getFileContent = useCallback(
    (projectId: string, filePath: string): Promise<FileContentResponse> =>
      wrap(async () => {
        if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/projects/${projectId}/files/${filePath}`,
        );
        if (!response.ok) {
          if (response.status === 404) throw new Error("File not found");
          const errorText = await response.text();
          throw new Error(errorText || "Failed to get file content");
        }
        return response.json();
      }),
    [workspace, wrap],
  );

  const updateFileContent = useCallback(
    (
      projectId: string,
      filePath: string,
      content: string,
    ): Promise<FileUpdateResponse> =>
      wrap(async () => {
        if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/projects/${projectId}/files/${filePath}`,
          {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ content }),
          },
        );
        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to update file");
        }
        const result = await response.json();
        invalidateProject(projectId);
        return result;
      }),
    [workspace, wrap, invalidateProject],
  );

  const createFile = useCallback(
    (
      projectId: string,
      parentPath: string | null,
      name: string,
      isDirectory: boolean,
      content?: string,
    ): Promise<FileCreateResponse> =>
      wrap(async () => {
        if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
        const url = parentPath
          ? `/api/workspaces/${workspace}/arena/projects/${projectId}/files/${parentPath}`
          : `/api/workspaces/${workspace}/arena/projects/${projectId}/files`;
        const response = await fetch(url, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ name, isDirectory, content }),
        });
        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to create file");
        }
        const result = await response.json();
        invalidateProject(projectId);
        return result;
      }),
    [workspace, wrap, invalidateProject],
  );

  const deleteFile = useCallback(
    (projectId: string, filePath: string): Promise<void> =>
      wrap(async () => {
        if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/projects/${projectId}/files/${filePath}`,
          { method: "DELETE" },
        );
        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to delete file");
        }
        invalidateProject(projectId);
      }),
    [workspace, wrap, invalidateProject],
  );

  const refreshFileTree = useCallback(
    (projectId: string): Promise<FileTreeNode[]> =>
      wrap(async () => {
        if (!workspace) throw new Error(NO_WORKSPACE_ERROR);
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/projects/${projectId}/files`,
        );
        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to refresh file tree");
        }
        const data = await response.json();
        return data.tree || [];
      }),
    [workspace, wrap],
  );

  return {
    getFileContent,
    updateFileContent,
    createFile,
    deleteFile,
    refreshFileTree,
    loading,
    error,
  };
}

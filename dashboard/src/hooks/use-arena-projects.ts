"use client";

import { useState, useEffect, useCallback } from "react";
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

// =============================================================================
// Project List Hook
// =============================================================================

interface UseArenaProjectsResult {
  projects: ArenaProject[];
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

/**
 * Hook to fetch Arena projects for the current workspace.
 */
export function useArenaProjects(): UseArenaProjectsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [projects, setProjects] = useState<ArenaProject[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace) {
      setProjects([]);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(`/api/workspaces/${workspace}/arena/projects`);
      if (!response.ok) {
        throw new Error(`Failed to fetch projects: ${response.statusText}`);
      }
      const data = await response.json();
      setProjects(data.projects || []);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setProjects([]);
    } finally {
      setLoading(false);
    }
  }, [workspace]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    projects,
    loading,
    error,
    refetch: fetchData,
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

/**
 * Hook to fetch a single Arena project with its file tree.
 */
export function useArenaProject(projectId: string | undefined): UseArenaProjectResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [project, setProject] = useState<ArenaProjectWithTree | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace || !projectId) {
      setProject(null);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(
        `/api/workspaces/${workspace}/arena/projects/${projectId}`
      );

      if (!response.ok) {
        if (response.status === 404) {
          throw new Error("Project not found");
        }
        throw new Error(`Failed to fetch project: ${response.statusText}`);
      }

      const projectData = await response.json();
      setProject(projectData);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setProject(null);
    } finally {
      setLoading(false);
    }
  }, [workspace, projectId]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    project,
    loading,
    error,
    refetch: fetchData,
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
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const createProject = useCallback(
    async (data: ProjectCreateRequest): Promise<ArenaProject> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
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

  const deleteProject = useCallback(
    async (projectId: string): Promise<void> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/projects/${projectId}`,
          { method: "DELETE" }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to delete project");
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
    createProject,
    deleteProject,
    loading,
    error,
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
 */
export function useArenaProjectFiles(): UseArenaProjectFilesResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const getFileContent = useCallback(
    async (projectId: string, filePath: string): Promise<FileContentResponse> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/projects/${projectId}/files/${filePath}`
        );

        if (!response.ok) {
          if (response.status === 404) {
            throw new Error("File not found");
          }
          const errorText = await response.text();
          throw new Error(errorText || "Failed to get file content");
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

  const updateFileContent = useCallback(
    async (projectId: string, filePath: string, content: string): Promise<FileUpdateResponse> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/projects/${projectId}/files/${filePath}`,
          {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ content }),
          }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to update file");
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

  const createFile = useCallback(
    async (
      projectId: string,
      parentPath: string | null,
      name: string,
      isDirectory: boolean,
      content?: string
    ): Promise<FileCreateResponse> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
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

  const deleteFile = useCallback(
    async (projectId: string, filePath: string): Promise<void> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/projects/${projectId}/files/${filePath}`,
          { method: "DELETE" }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to delete file");
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

  const refreshFileTree = useCallback(
    async (projectId: string): Promise<FileTreeNode[]> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/projects/${projectId}/files`
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to refresh file tree");
        }

        const data = await response.json();
        return data.tree || [];
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
    getFileContent,
    updateFileContent,
    createFile,
    deleteFile,
    refreshFileTree,
    loading,
    error,
  };
}

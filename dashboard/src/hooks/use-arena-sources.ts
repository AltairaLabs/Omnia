"use client";

import { useState, useEffect, useCallback } from "react";
import { useWorkspace } from "@/contexts/workspace-context";
import type { ArenaSource } from "@/types/arena";

const NO_WORKSPACE_ERROR = "No workspace selected";

interface UseArenaSourcesResult {
  sources: ArenaSource[];
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

interface UseArenaSourceResult {
  source: ArenaSource | null;
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

interface UseArenaSourceMutationsResult {
  createSource: (name: string, spec: ArenaSource["spec"]) => Promise<ArenaSource>;
  updateSource: (name: string, spec: ArenaSource["spec"]) => Promise<ArenaSource>;
  deleteSource: (name: string) => Promise<void>;
  syncSource: (name: string) => Promise<void>;
  loading: boolean;
  error: Error | null;
}

/**
 * Hook to fetch Arena sources for the current workspace.
 */
export function useArenaSources(): UseArenaSourcesResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [sources, setSources] = useState<ArenaSource[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace) {
      setSources([]);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(`/api/workspaces/${workspace}/arena/sources`);
      if (!response.ok) {
        throw new Error(`Failed to fetch sources: ${response.statusText}`);
      }
      const data = await response.json();
      setSources(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setSources([]);
    } finally {
      setLoading(false);
    }
  }, [workspace]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    sources,
    loading,
    error,
    refetch: fetchData,
  };
}

/**
 * Hook to fetch a single Arena source.
 */
export function useArenaSource(name: string | undefined): UseArenaSourceResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [source, setSource] = useState<ArenaSource | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace || !name) {
      setSource(null);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(`/api/workspaces/${workspace}/arena/sources/${name}`);

      if (!response.ok) {
        if (response.status === 404) {
          throw new Error("Source not found");
        }
        throw new Error(`Failed to fetch source: ${response.statusText}`);
      }

      const sourceData = await response.json();
      setSource(sourceData);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setSource(null);
    } finally {
      setLoading(false);
    }
  }, [workspace, name]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    source,
    loading,
    error,
    refetch: fetchData,
  };
}

/**
 * Hook to provide mutations for Arena sources (create, update, delete, sync).
 */
export function useArenaSourceMutations(): UseArenaSourceMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const createSource = useCallback(
    async (name: string, spec: ArenaSource["spec"]): Promise<ArenaSource> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(`/api/workspaces/${workspace}/arena/sources`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ metadata: { name }, spec }),
        });

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to create source");
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

  const updateSource = useCallback(
    async (name: string, spec: ArenaSource["spec"]): Promise<ArenaSource> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/sources/${name}`,
          {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ spec }),
          }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to update source");
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

  const deleteSource = useCallback(
    async (name: string): Promise<void> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/sources/${name}`,
          { method: "DELETE" }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to delete source");
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

  const syncSource = useCallback(
    async (name: string): Promise<void> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/sources/${name}/sync`,
          { method: "POST" }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to sync source");
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
    createSource,
    updateSource,
    deleteSource,
    syncSource,
    loading,
    error,
  };
}

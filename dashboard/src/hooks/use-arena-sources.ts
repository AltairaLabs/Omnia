"use client";

/**
 * Arena Sources hooks for fetching and mutating Arena sources.
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
import type { ArenaSource, ArenaConfig } from "@/types/arena";

const NO_WORKSPACE_ERROR = "No workspace selected";

interface UseArenaSourcesResult {
  sources: ArenaSource[];
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

interface UseArenaSourceResult {
  source: ArenaSource | null;
  linkedConfigs: ArenaConfig[];
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
 *
 * Uses DataService for demo/live mode support.
 */
export function useArenaSources(): UseArenaSourcesResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const service = useDataService();
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
      const data = await service.getArenaSources(workspace);
      setSources(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setSources([]);
    } finally {
      setLoading(false);
    }
  }, [workspace, service]);

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
 * Hook to fetch a single Arena source and its linked configs.
 *
 * Uses DataService for demo/live mode support.
 */
export function useArenaSource(name: string | undefined): UseArenaSourceResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const service = useDataService();
  const [source, setSource] = useState<ArenaSource | null>(null);
  const [linkedConfigs, setLinkedConfigs] = useState<ArenaConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace || !name) {
      setSource(null);
      setLinkedConfigs([]);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      // Fetch source and configs in parallel
      const [sourceData, configsData] = await Promise.all([
        service.getArenaSource(workspace, name),
        service.getArenaConfigs(workspace),
      ]);

      if (!sourceData) {
        throw new Error("Source not found");
      }

      setSource(sourceData);

      // Filter configs that reference this source
      const linked = configsData.filter(
        (config) => config.spec?.sourceRef?.name === name
      );
      setLinkedConfigs(linked);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setSource(null);
      setLinkedConfigs([]);
    } finally {
      setLoading(false);
    }
  }, [workspace, name, service]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    source,
    linkedConfigs,
    loading,
    error,
    refetch: fetchData,
  };
}

/**
 * Hook to provide mutations for Arena sources (create, update, delete, sync).
 *
 * Uses DataService for demo/live mode support.
 */
export function useArenaSourceMutations(): UseArenaSourceMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const service = useDataService();
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
        return await service.createArenaSource(workspace, name, spec);
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

  const updateSource = useCallback(
    async (name: string, spec: ArenaSource["spec"]): Promise<ArenaSource> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        return await service.updateArenaSource(workspace, name, spec);
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

  const deleteSource = useCallback(
    async (name: string): Promise<void> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        await service.deleteArenaSource(workspace, name);
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

  const syncSource = useCallback(
    async (name: string): Promise<void> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        await service.syncArenaSource(workspace, name);
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
    createSource,
    updateSource,
    deleteSource,
    syncSource,
    loading,
    error,
  };
}

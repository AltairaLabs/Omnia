"use client";

/**
 * Arena Configs hooks for fetching and mutating Arena configs.
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
import type { ArenaConfig, ArenaJob, Scenario } from "@/types/arena";

const NO_WORKSPACE_ERROR = "No workspace selected";

interface UseArenaConfigsResult {
  configs: ArenaConfig[];
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

interface UseArenaConfigResult {
  config: ArenaConfig | null;
  scenarios: Scenario[];
  linkedJobs: ArenaJob[];
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

interface UseArenaConfigMutationsResult {
  createConfig: (name: string, spec: ArenaConfig["spec"]) => Promise<ArenaConfig>;
  updateConfig: (name: string, spec: ArenaConfig["spec"]) => Promise<ArenaConfig>;
  deleteConfig: (name: string) => Promise<void>;
  loading: boolean;
  error: Error | null;
}

/**
 * Hook to fetch Arena configs for the current workspace.
 *
 * Uses DataService for demo/live mode support.
 */
export function useArenaConfigs(): UseArenaConfigsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const service = useDataService();
  const [configs, setConfigs] = useState<ArenaConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace) {
      setConfigs([]);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const data = await service.getArenaConfigs(workspace);
      setConfigs(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setConfigs([]);
    } finally {
      setLoading(false);
    }
  }, [workspace, service]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    configs,
    loading,
    error,
    refetch: fetchData,
  };
}

/**
 * Hook to fetch a single Arena config with its scenarios and linked jobs.
 *
 * Uses DataService for demo/live mode support.
 */
export function useArenaConfig(name: string | undefined): UseArenaConfigResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const service = useDataService();
  const [config, setConfig] = useState<ArenaConfig | null>(null);
  const [scenarios, setScenarios] = useState<Scenario[]>([]);
  const [linkedJobs, setLinkedJobs] = useState<ArenaJob[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const fetchData = useCallback(async () => {
    if (!workspace || !name) {
      setConfig(null);
      setScenarios([]);
      setLinkedJobs([]);
      setLoading(false);
      return;
    }

    setLoading(true);
    setError(null);

    try {
      // Fetch config, scenarios, and jobs in parallel
      const [configData, scenariosData, jobsData] = await Promise.all([
        service.getArenaConfig(workspace, name),
        service.getArenaConfigScenarios(workspace, name),
        service.getArenaJobs(workspace, { configRef: name }),
      ]);

      if (!configData) {
        throw new Error("Config not found");
      }

      setConfig(configData);
      setScenarios(scenariosData);
      setLinkedJobs(jobsData);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setConfig(null);
      setScenarios([]);
      setLinkedJobs([]);
    } finally {
      setLoading(false);
    }
  }, [workspace, name, service]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    config,
    scenarios,
    linkedJobs,
    loading,
    error,
    refetch: fetchData,
  };
}

/**
 * Hook to provide mutations for Arena configs (create, update, delete).
 *
 * Uses DataService for demo/live mode support.
 */
export function useArenaConfigMutations(): UseArenaConfigMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const service = useDataService();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const createConfig = useCallback(
    async (name: string, spec: ArenaConfig["spec"]): Promise<ArenaConfig> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        return await service.createArenaConfig(workspace, name, spec);
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

  const updateConfig = useCallback(
    async (name: string, spec: ArenaConfig["spec"]): Promise<ArenaConfig> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        return await service.updateArenaConfig(workspace, name, spec);
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

  const deleteConfig = useCallback(
    async (name: string): Promise<void> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        await service.deleteArenaConfig(workspace, name);
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
    createConfig,
    updateConfig,
    deleteConfig,
    loading,
    error,
  };
}

"use client";

import { useState, useEffect, useCallback } from "react";
import { useWorkspace } from "@/contexts/workspace-context";
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
 */
export function useArenaConfigs(): UseArenaConfigsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
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
      const response = await fetch(`/api/workspaces/${workspace}/arena/configs`);
      if (!response.ok) {
        throw new Error(`Failed to fetch configs: ${response.statusText}`);
      }
      const data = await response.json();
      setConfigs(data);
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setConfigs([]);
    } finally {
      setLoading(false);
    }
  }, [workspace]);

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
 */
export function useArenaConfig(name: string | undefined): UseArenaConfigResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
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
      const [configResponse, scenariosResponse, jobsResponse] = await Promise.all([
        fetch(`/api/workspaces/${workspace}/arena/configs/${name}`),
        fetch(`/api/workspaces/${workspace}/arena/configs/${name}/scenarios`),
        fetch(`/api/workspaces/${workspace}/arena/jobs?configRef=${name}`),
      ]);

      if (!configResponse.ok) {
        if (configResponse.status === 404) {
          throw new Error("Config not found");
        }
        throw new Error(`Failed to fetch config: ${configResponse.statusText}`);
      }

      const configData = await configResponse.json();
      setConfig(configData);

      if (scenariosResponse.ok) {
        const scenariosData: Scenario[] = await scenariosResponse.json();
        setScenarios(scenariosData);
      } else {
        setScenarios([]);
      }

      if (jobsResponse.ok) {
        const jobsData: ArenaJob[] = await jobsResponse.json();
        setLinkedJobs(jobsData);
      } else {
        setLinkedJobs([]);
      }
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setConfig(null);
      setScenarios([]);
      setLinkedJobs([]);
    } finally {
      setLoading(false);
    }
  }, [workspace, name]);

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
 */
export function useArenaConfigMutations(): UseArenaConfigMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
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
        const response = await fetch(`/api/workspaces/${workspace}/arena/configs`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ metadata: { name }, spec }),
        });

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to create config");
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

  const updateConfig = useCallback(
    async (name: string, spec: ArenaConfig["spec"]): Promise<ArenaConfig> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/configs/${name}`,
          {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ spec }),
          }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to update config");
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

  const deleteConfig = useCallback(
    async (name: string): Promise<void> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(
          `/api/workspaces/${workspace}/arena/configs/${name}`,
          { method: "DELETE" }
        );

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to delete config");
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
    createConfig,
    updateConfig,
    deleteConfig,
    loading,
    error,
  };
}

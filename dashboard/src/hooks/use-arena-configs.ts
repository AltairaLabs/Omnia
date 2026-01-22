"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";
import { useWorkspace } from "@/contexts/workspace-context";
import type { ArenaConfig, ArenaConfigContent, ArenaJob, Scenario } from "@/types/arena";

const NO_WORKSPACE_ERROR = "No workspace selected";
const QUERY_KEY_ARENA_CONFIGS = "arena-configs";
const QUERY_KEY_ARENA_CONFIG = "arena-config";

/** Convert unknown error to Error type */
function toError(error: unknown): Error | null {
  if (!error) return null;
  if (error instanceof Error) return error;
  return new Error(String(error));
}

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
 * Uses the DataService to support both mock (demo) and live modes.
 */
export function useArenaConfigs(): UseArenaConfigsResult {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: [QUERY_KEY_ARENA_CONFIGS, workspace, service.name],
    queryFn: async (): Promise<ArenaConfig[]> => {
      if (!workspace) {
        return [];
      }
      return service.getArenaConfigs(workspace) as Promise<ArenaConfig[]>;
    },
    enabled: !!workspace,
    staleTime: 0,
    refetchOnMount: "always",
  });

  return {
    configs: data ?? [],
    loading: isLoading,
    error: toError(error),
    refetch: () => { refetch(); },
  };
}

/**
 * Hook to fetch a single Arena config with its scenarios and linked jobs.
 * Uses the DataService to support both mock (demo) and live modes.
 */
export function useArenaConfig(name: string | undefined): UseArenaConfigResult {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: [QUERY_KEY_ARENA_CONFIG, workspace, name, service.name],
    queryFn: async () => {
      if (!workspace || !name) {
        return { config: null, scenarios: [], linkedJobs: [] };
      }

      // Fetch config, scenarios, and jobs in parallel using the data service
      const [config, scenarios, linkedJobs] = await Promise.all([
        service.getArenaConfig(workspace, name),
        service.getArenaConfigScenarios(workspace, name),
        service.getArenaJobs(workspace, { configRef: name }),
      ]);

      if (!config) {
        throw new Error("Config not found");
      }

      return {
        config: config as ArenaConfig,
        scenarios: scenarios as Scenario[],
        linkedJobs: linkedJobs as ArenaJob[],
      };
    },
    enabled: !!workspace && !!name,
    staleTime: 0,
    refetchOnMount: "always",
  });

  return {
    config: data?.config ?? null,
    scenarios: data?.scenarios ?? [],
    linkedJobs: data?.linkedJobs ?? [],
    loading: isLoading,
    error: toError(error),
    refetch: () => { refetch(); },
  };
}

/**
 * Hook to provide mutations for Arena configs (create, update, delete).
 * Uses the DataService to support both mock (demo) and live modes.
 */
export function useArenaConfigMutations(): UseArenaConfigMutationsResult {
  const service = useDataService();
  const queryClient = useQueryClient();
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const createMutation = useMutation({
    mutationFn: async ({ name, spec }: { name: string; spec: ArenaConfig["spec"] }): Promise<ArenaConfig> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }
      return service.createArenaConfig(workspace, name, spec) as Promise<ArenaConfig>;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEY_ARENA_CONFIGS, workspace] });
    },
  });

  const updateMutation = useMutation({
    mutationFn: async ({ name, spec }: { name: string; spec: ArenaConfig["spec"] }): Promise<ArenaConfig> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }
      return service.updateArenaConfig(workspace, name, spec) as Promise<ArenaConfig>;
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEY_ARENA_CONFIGS, workspace] });
      queryClient.invalidateQueries({ queryKey: [QUERY_KEY_ARENA_CONFIG, workspace, variables.name] });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: async (name: string): Promise<void> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }
      return service.deleteArenaConfig(workspace, name);
    },
    onSuccess: (_, name) => {
      queryClient.invalidateQueries({ queryKey: [QUERY_KEY_ARENA_CONFIGS, workspace] });
      queryClient.invalidateQueries({ queryKey: [QUERY_KEY_ARENA_CONFIG, workspace, name] });
    },
  });

  // Combine loading and error states from all mutations
  const loading = createMutation.isPending || updateMutation.isPending || deleteMutation.isPending;
  const error = createMutation.error || updateMutation.error || deleteMutation.error;

  return {
    createConfig: (name: string, spec: ArenaConfig["spec"]) => createMutation.mutateAsync({ name, spec }),
    updateConfig: (name: string, spec: ArenaConfig["spec"]) => updateMutation.mutateAsync({ name, spec }),
    deleteConfig: (name: string) => deleteMutation.mutateAsync(name),
    loading,
    error: toError(error),
  };
}

const QUERY_KEY_ARENA_CONFIG_CONTENT = "arena-config-content";

interface UseArenaConfigContentResult {
  content: ArenaConfigContent | null;
  loading: boolean;
  error: Error | null;
  refetch: () => void;
}

/**
 * Hook to fetch parsed pack content for an Arena config.
 * Uses the DataService to support both mock (demo) and live modes.
 * Returns prompts, tools, and scenarios from the pack.
 */
export function useArenaConfigContent(name: string | undefined): UseArenaConfigContentResult {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: [QUERY_KEY_ARENA_CONFIG_CONTENT, workspace, name, service.name],
    queryFn: async (): Promise<ArenaConfigContent | null> => {
      if (!workspace || !name) {
        return null;
      }
      return service.getArenaConfigContent(workspace, name);
    },
    enabled: !!workspace && !!name,
    staleTime: 30000, // Cache for 30 seconds since pack content rarely changes
    refetchOnMount: true,
  });

  return {
    content: data ?? null,
    loading: isLoading,
    error: toError(error),
    refetch: () => { refetch(); },
  };
}

const QUERY_KEY_ARENA_CONFIG_FILE = "arena-config-file";

interface UseArenaConfigFileResult {
  content: string | null;
  loading: boolean;
  error: Error | null;
}

/**
 * Hook to fetch individual file content from an Arena config.
 * Fetches file content on-demand when a path is selected.
 */
export function useArenaConfigFile(
  configName: string | undefined,
  filePath: string | null
): UseArenaConfigFileResult {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const { data, isLoading, error } = useQuery({
    queryKey: [QUERY_KEY_ARENA_CONFIG_FILE, workspace, configName, filePath, service.name],
    queryFn: async (): Promise<string | null> => {
      if (!workspace || !configName || !filePath) {
        return null;
      }
      return service.getArenaConfigFile(workspace, configName, filePath);
    },
    enabled: !!workspace && !!configName && !!filePath,
    staleTime: 60000, // Cache for 60 seconds
  });

  return {
    content: data ?? null,
    loading: isLoading,
    error: toError(error),
  };
}

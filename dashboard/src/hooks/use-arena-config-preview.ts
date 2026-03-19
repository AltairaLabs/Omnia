"use client";

import { useState, useEffect, useCallback } from "react";
import { useWorkspace } from "@/contexts/workspace-context";
import yaml from "js-yaml";

/** A provider ID referenced by the arena config (judges, self-play roles, etc.) */
export interface ConfigProviderRef {
  /** The provider ID as referenced in the config */
  id: string;
  /** Source context: which config section references this provider */
  source: "judges" | "judge_specs" | "self_play";
  /** Human-readable label for display */
  label: string;
}

export interface ArenaConfigPreview {
  /** Number of scenarios declared in the arena config */
  scenarioCount: number;
  /** Number of providers declared in the arena config */
  configProviderCount: number;
  /** Provider groups required by the arena config (from provider file groups + self-play roles) */
  requiredGroups: string[];
  /** Provider IDs referenced by judges, judge_specs, and self-play roles */
  providerRefs: ConfigProviderRef[];
  /** Whether the config was successfully parsed */
  loaded: boolean;
  /** Whether the config is being fetched */
  loading: boolean;
  /** Error message if fetch/parse failed */
  error: string | null;
}

/**
 * Minimal shape of the arena config YAML we need for preview.
 * Extracts scenario counts, provider groups, and self-play role references.
 */
interface ArenaConfigYaml {
  spec?: {
    scenarios?: { file?: string }[];
    providers?: { file?: string; name?: string; group?: string }[];
    self_play?: {
      enabled?: boolean;
      roles?: { id?: string; provider?: string }[];
    };
    judges?: { name?: string; provider?: string }[];
    judge_specs?: Record<string, { provider?: string }>;
  };
}

/**
 * Extract provider ID references from judges, judge_specs, and self-play roles.
 * These are provider IDs the config expects to exist — map-mode groups use them as keys.
 */
function extractProviderRefs(parsed: ArenaConfigYaml): ConfigProviderRef[] {
  const refs: ConfigProviderRef[] = [];
  const seen = new Set<string>();

  const add = (id: string, source: ConfigProviderRef["source"], label: string) => {
    const key = `${source}:${id}`;
    if (id && !seen.has(key)) {
      seen.add(key);
      refs.push({ id, source, label });
    }
  };

  for (const role of parsed?.spec?.self_play?.roles ?? []) {
    if (role.provider) {
      add(role.provider, "self_play", `Self-play role "${role.id || "unknown"}"`);
    }
  }

  for (const judge of parsed?.spec?.judges ?? []) {
    if (judge.provider) {
      add(judge.provider, "judges", `Judge "${judge.name || "unknown"}"`);
    }
  }

  for (const [name, spec] of Object.entries(parsed?.spec?.judge_specs ?? {})) {
    if (spec.provider) {
      add(spec.provider, "judge_specs", `Judge spec "${name}"`);
    }
  }

  return refs;
}

/**
 * Extract the unique provider group names from spec.providers[].group.
 * Only test-matrix provider groups belong here — self-play, judge, and
 * judge_spec provider ID references are handled via providerRefs/map-mode.
 */
function extractRequiredGroups(parsed: ArenaConfigYaml): string[] {
  const groups = new Set<string>();
  for (const p of parsed?.spec?.providers ?? []) {
    groups.add(p.group || "default");
  }
  return Array.from(groups);
}

/**
 * Fetches and parses an arena config YAML file to extract scenario and provider counts.
 * Used to calculate optimal worker count in the job wizard.
 *
 * @param sourceName - ArenaSource name to read from
 * @param configPath - Relative path to the arena config file (e.g., "config.arena.yaml")
 */
export function useArenaConfigPreview(
  sourceName: string | undefined,
  configPath: string | undefined
): ArenaConfigPreview {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const emptyState: ArenaConfigPreview = {
    scenarioCount: 0,
    configProviderCount: 0,
    requiredGroups: [],
    providerRefs: [],
    loaded: false,
    loading: false,
    error: null,
  };

  const [state, setState] = useState<ArenaConfigPreview>(emptyState);

  const fetchConfig = useCallback(async () => {
    if (!workspace || !sourceName || !configPath) {
      setState(emptyState);
      return;
    }

    setState((prev) => ({ ...prev, loading: true, error: null }));

    try {
      const url = `/api/workspaces/${encodeURIComponent(workspace)}/arena/sources/${encodeURIComponent(sourceName)}/file?path=${encodeURIComponent(configPath)}`;
      const response = await fetch(url);

      if (!response.ok) {
        if (response.status === 404) {
          setState(emptyState);
          return;
        }
        throw new Error(`Failed to fetch config: ${response.statusText}`);
      }

      const data = await response.json();
      const parsed = yaml.load(data.content) as ArenaConfigYaml;

      const scenarios = parsed?.spec?.scenarios ?? [];
      const providers = parsed?.spec?.providers ?? [];
      const scenarioCount = scenarios.filter((s) => s?.file || s).length;
      const configProviderCount = providers.filter((p) => p?.file || p?.name || p).length;
      const requiredGroups = extractRequiredGroups(parsed);
      const providerRefs = extractProviderRefs(parsed);

      setState({
        scenarioCount,
        configProviderCount,
        requiredGroups,
        providerRefs,
        loaded: true,
        loading: false,
        error: null,
      });
    } catch (err) {
      setState({
        ...emptyState,
        error: err instanceof Error ? err.message : String(err),
      });
    }
  }, [workspace, sourceName, configPath]);

  useEffect(() => {
    fetchConfig();
  }, [fetchConfig]);

  return state;
}

export interface WorkItemEstimate {
  /** Total work items that will be created */
  workItems: number;
  /** Recommended worker count (= workItems, capped at maxWorkerReplicas if set) */
  recommendedWorkers: number;
  /** Human-readable explanation of the work item calculation */
  description: string;
}

/**
 * Calculates the estimated number of work items and recommended worker count.
 *
 * Work items = scenarios x totalProviderEntries (from CRD-based provider groups).
 * If no providers are specified, falls back to 1 work item.
 * If scenarios are not yet known, uses totalProviderEntries alone.
 *
 * @param config - Parsed arena config preview with scenario/provider counts
 * @param totalProviderEntries - Total number of provider/agent entries across all groups
 * @param maxWorkerReplicas - Maximum allowed worker replicas (0 = unlimited)
 */
export function estimateWorkItems(
  config: ArenaConfigPreview,
  totalProviderEntries: number,
  maxWorkerReplicas: number
): WorkItemEstimate {
  if (!config.loaded) {
    return {
      workItems: 1,
      recommendedWorkers: 1,
      description: "",
    };
  }

  const providers = Math.max(totalProviderEntries, 0);
  const plural = (n: number) => (n === 1 ? "" : "s");
  let workItems: number;
  let description: string;

  if (providers === 0) {
    // No providers specified -- single fallback work item
    workItems = 1;
    description = "1 work item (no providers specified)";
  } else if (config.scenarioCount > 0) {
    workItems = config.scenarioCount * providers;
    description =
      `${config.scenarioCount} scenario${plural(config.scenarioCount)} \u00d7 ${providers} provider${plural(providers)}`;
  } else {
    // Scenarios will be enumerated at runtime from the arena file
    workItems = providers;
    description =
      `${providers} provider${plural(providers)} (scenarios enumerated at runtime)`;
  }

  let recommendedWorkers = workItems;
  if (maxWorkerReplicas > 0) {
    recommendedWorkers = Math.min(recommendedWorkers, maxWorkerReplicas);
  }

  return { workItems, recommendedWorkers, description };
}

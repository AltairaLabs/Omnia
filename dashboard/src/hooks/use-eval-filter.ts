/**
 * Hook for discovering eval filter values and managing filter state.
 *
 * Previously discovered `agent` and `promptpack_name` label values from
 * Prometheus eval gauges; migrated to the structured session-api
 * `/api/workspaces/{name}/eval-results/discover` endpoint as the third
 * wave of the observability split — see CLAUDE.md → Observability
 * Boundaries and
 * `docs/local-backlog/implemented/2026-04-17-observability-split-design.md`.
 *
 * Public surface (`UseEvalFilterResult`, `useEvalFilter`) is unchanged so
 * the `/quality` page consumes it without modification.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useState, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import { fetchEvalDiscovery } from "@/lib/data/eval-results-service";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";

/**
 * Filter shape mirrors the prom-era EvalFilter so /quality and chart
 * consumers can pass it through unchanged.
 */
export interface EvalFilter {
  agent?: string;
  promptpackName?: string;
}

export interface UseEvalFilterResult {
  agents: string[];
  promptpacks: string[];
  selectedAgent: string | undefined;
  selectedPromptPack: string | undefined;
  setAgent: (agent: string | undefined) => void;
  setPromptPack: (pp: string | undefined) => void;
  filter: EvalFilter;
  isLoading: boolean;
}

export function useEvalFilter(): UseEvalFilterResult {
  const [selectedAgent, setAgent] = useState<string | undefined>();
  const [selectedPromptPack, setPromptPack] = useState<string | undefined>();
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  const discoveryQuery = useQuery({
    queryKey: ["eval-filter-discovery", workspace],
    enabled: Boolean(workspace),
    queryFn: async () => {
      if (!workspace) return { evals: [], agents: [], promptpacks: [] };
      return fetchEvalDiscovery(workspace);
    },
    staleTime: DEFAULT_STALE_TIME,
    retry: false,
  });

  const filter = useMemo<EvalFilter>(() => {
    const f: EvalFilter = {};
    if (selectedAgent) f.agent = selectedAgent;
    if (selectedPromptPack) f.promptpackName = selectedPromptPack;
    return f;
  }, [selectedAgent, selectedPromptPack]);

  return {
    agents: discoveryQuery.data?.agents ?? [],
    promptpacks: discoveryQuery.data?.promptpacks ?? [],
    selectedAgent,
    selectedPromptPack,
    setAgent,
    setPromptPack,
    filter,
    isLoading: discoveryQuery.isLoading,
  };
}

/**
 * Hook for discovering eval label values and managing filter state.
 *
 * Queries Prometheus to discover available agent and promptpack_name
 * label values from omnia_eval_* metrics.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useState, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { queryPrometheus, type PrometheusVectorResult } from "@/lib/prometheus";
import { EvalQueries, type EvalFilter } from "@/lib/prometheus-queries";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";

/** Extract unique non-empty label values from a Prometheus group-by result. */
function extractLabelValues(
  result: PrometheusVectorResult[] | undefined,
  labelKey: string,
): string[] {
  if (!result) return [];
  const values = new Set<string>();
  for (const item of result) {
    const val = item.metric[labelKey];
    if (val) values.add(val);
  }
  return Array.from(values).sort((a, b) => a.localeCompare(b));
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

  const agentsQuery = useQuery({
    queryKey: ["eval-filter-agents"],
    queryFn: async () => {
      const resp = await queryPrometheus(EvalQueries.discoverAgents());
      if (resp.status !== "success" || !resp.data?.result) return [];
      return extractLabelValues(resp.data.result, "agent");
    },
    staleTime: DEFAULT_STALE_TIME,
    retry: false,
  });

  const promptpacksQuery = useQuery({
    queryKey: ["eval-filter-promptpacks"],
    queryFn: async () => {
      const resp = await queryPrometheus(EvalQueries.discoverPromptPacks());
      if (resp.status !== "success" || !resp.data?.result) return [];
      return extractLabelValues(resp.data.result, "promptpack_name");
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
    agents: agentsQuery.data ?? [],
    promptpacks: promptpacksQuery.data ?? [],
    selectedAgent,
    selectedPromptPack,
    setAgent,
    setPromptPack,
    filter,
    isLoading: agentsQuery.isLoading || promptpacksQuery.isLoading,
  };
}

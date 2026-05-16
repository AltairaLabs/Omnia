/**
 * Hook for the agent quality dashboard's per-eval score summary.
 *
 * Previously read from Prometheus eval gauge metrics; migrated to the
 * structured session-api read path (`/api/workspaces/{name}/eval-results/
 * aggregate` + `.../discover`) as the second wave of the observability
 * split — see CLAUDE.md → Observability Boundaries and
 * `docs/local-backlog/implemented/2026-04-17-observability-split-design.md`.
 *
 * Public surface (`EvalScoreSummary`, `useEvalSummary`) is unchanged so the
 * `/quality` page consumes it without modification.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import {
  fetchEvalAggregate,
  fetchEvalDescriptors,
  classifyEvalType,
} from "@/lib/data/eval-results-service";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";
import type { EvalMetricType } from "@/types/eval";

/**
 * Filter shape mirrors the prom-era EvalFilter so the `/quality` page can
 * pass it through unchanged. Keeping it locally-defined avoids importing
 * from `@/lib/prometheus-queries` from a now-prom-free hook.
 */
export interface EvalFilter {
  agent?: string;
  promptpackName?: string;
}

export interface EvalScoreSummary {
  evalId: string;
  score: number;
  metricType: EvalMetricType;
}

/**
 * Fetch eval-score summaries (one row per eval) for the current workspace.
 *
 * Implementation:
 * 1. Discover the (eval_id, eval_type) set for this workspace.
 * 2. For each eval, request `avg_score` collapsed to a single row.
 * 3. Map to the EvalScoreSummary shape; default to 0 when there are no
 *    scored rows yet (boolean evals return 1.0 / 0.0 from avg).
 */
export function useEvalSummary(filter?: EvalFilter) {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  return useQuery({
    queryKey: ["eval-summary", workspace, filter],
    enabled: Boolean(workspace),
    queryFn: async (): Promise<EvalScoreSummary[]> => {
      if (!workspace) return [];

      const descriptors = await fetchEvalDescriptors(workspace);
      if (descriptors.length === 0) return [];

      const summaries = await Promise.all(
        descriptors.map(async (desc) => {
          const rows = await fetchEvalAggregate({
            workspace,
            groupBy: "eval_id",
            metric: "avg_score",
            evalId: desc.evalId,
            agentName: filter?.agent,
            promptpackName: filter?.promptpackName,
          });
          return {
            evalId: desc.evalId,
            score: rows[0]?.value ?? 0,
            metricType: classifyEvalType(desc.evalType),
          };
        }),
      );

      // Sort by eval_id for stable card order (matches the previous Prom
      // path's alphabetical sort).
      return summaries.sort((a, b) => a.evalId.localeCompare(b.evalId));
    },
    staleTime: DEFAULT_STALE_TIME,
    retry: false,
  });
}

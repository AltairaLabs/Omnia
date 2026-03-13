/**
 * Hooks for agent quality dashboard with eval scores.
 *
 * useEvalSummary reads from Prometheus (eval gauge metrics) and returns
 * score-only summaries (no pass/fail concepts).
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useQuery } from "@tanstack/react-query";
import { queryPrometheus } from "@/lib/prometheus";
import { EvalQueries, type EvalFilter } from "@/lib/prometheus-queries";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";
import { discoverEvalMetrics } from "@/lib/eval-discovery";
import type { EvalMetricType } from "@/types/eval";

export interface EvalScoreSummary {
  evalId: string;
  score: number;
  metricType: EvalMetricType;
}

/**
 * Fetch eval summary (scores per eval) from Prometheus metrics.
 *
 * Discovers omnia_eval_* metrics (with types resolved by metadata),
 * fetches their current values, and returns EvalScoreSummary[].
 */
export function useEvalSummary(filter?: EvalFilter) {
  return useQuery({
    queryKey: ["eval-summary", filter],
    queryFn: async (): Promise<EvalScoreSummary[]> => {
      const discovered = await discoverEvalMetrics(filter);
      if (discovered.length === 0) return [];

      const valueResults = await Promise.all(
        discovered.map(async (metric) => {
          const query = EvalQueries.metricValue(metric.name, filter);
          const resp = await queryPrometheus(query);
          const value =
            resp.status === "success" && resp.data?.result?.[0]?.value
              ? Number.parseFloat(resp.data.result[0].value[1]) || 0
              : 0;
          return { ...metric, value };
        }),
      );

      return valueResults.map((r) => ({
        evalId: r.name.replace(/^omnia_eval_/, ""),
        score: r.value,
        metricType: r.metricType,
      }));
    },
    staleTime: DEFAULT_STALE_TIME,
    retry: false,
  });
}

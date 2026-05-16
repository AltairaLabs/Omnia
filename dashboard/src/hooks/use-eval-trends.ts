/**
 * Hooks for fetching eval-metric trends from session-api.
 *
 * Previously read from Prometheus. Migrated to the structured read path
 * (`/api/workspaces/{name}/eval-results/aggregate` + `.../discover`) as
 * the pilot for the observability split — see CLAUDE.md → Observability
 * Boundaries and
 * `docs/local-backlog/implemented/2026-04-17-observability-split-design.md`.
 *
 * The exported surface (EVAL_TREND_RANGES, EvalTrendPoint, EvalMetricInfo,
 * useEvalScoreTrends, useEvalMetrics) is unchanged so existing consumers
 * (`components/quality/eval-score-trend-chart.tsx`,
 * `components/quality/eval-score-breakdown.tsx`) don't need to change.
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
  type EvalAggregateGroupBy,
} from "@/lib/data/eval-results-service";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";
import type { EvalMetricType } from "@/types/eval";

/**
 * Filter for eval queries by dimensional values. Kept structurally
 * compatible with the previous EvalFilter from `@/lib/prometheus-queries`
 * so existing consumers can re-use the same shape verbatim.
 */
export interface EvalFilter {
  agent?: string;
  promptpackName?: string;
}

/** Time range options for trend queries. */
export const EVAL_TREND_RANGES = {
  "1h": { seconds: 3600, groupBy: "time:hour" as EvalAggregateGroupBy },
  "6h": { seconds: 21600, groupBy: "time:hour" as EvalAggregateGroupBy },
  "24h": { seconds: 86400, groupBy: "time:hour" as EvalAggregateGroupBy },
  "7d": { seconds: 604800, groupBy: "time:day" as EvalAggregateGroupBy },
  "30d": { seconds: 2592000, groupBy: "time:day" as EvalAggregateGroupBy },
} as const;

export type EvalTrendRange = keyof typeof EVAL_TREND_RANGES;

export interface EvalTrendPoint {
  timestamp: Date;
  /** Eval name → metric value at this timestamp. */
  values: Record<string, number>;
}

export interface EvalMetricInfo {
  name: string;
  value: number;
  metricType: EvalMetricType;
  sparkline?: Array<{ value: number }>;
}

/** Sparkline range — last 1h at hour-precision buckets. */
const SPARKLINE_RANGE = { seconds: 3600, groupBy: "time:hour" as EvalAggregateGroupBy };

/**
 * Fetch eval score trends for the current workspace.
 *
 * Behaviour:
 * 1. If no metric names are provided, discover them first from session-api.
 * 2. For each eval, fetch a time-bucketed avg_score series.
 * 3. Merge into the EvalTrendPoint[] shape callers expect.
 */
export function useEvalScoreTrends(params?: {
  metricNames?: string[];
  timeRange?: EvalTrendRange;
  filter?: EvalFilter;
}) {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const timeRange = params?.timeRange ?? "24h";
  const metricNames = params?.metricNames;
  const filter = params?.filter;
  const rangeConfig = EVAL_TREND_RANGES[timeRange];

  return useQuery({
    queryKey: ["eval-trends", workspace, metricNames, timeRange, filter],
    enabled: Boolean(workspace),
    queryFn: async (): Promise<EvalTrendPoint[]> => {
      if (!workspace) return [];

      const end = new Date();
      const start = new Date(end.getTime() - rangeConfig.seconds * 1000);

      let names: string[];
      if (metricNames && metricNames.length > 0) {
        names = metricNames;
      } else {
        const descriptors = await fetchEvalDescriptors(workspace);
        names = descriptors.map((d) => d.evalId);
      }
      if (names.length === 0) return [];

      const seriesPerEval = await Promise.all(
        names.map(async (name) => {
          const rows = await fetchEvalAggregate({
            workspace,
            groupBy: rangeConfig.groupBy,
            metric: "avg_score",
            evalId: name,
            agentName: filter?.agent,
            promptpackName: filter?.promptpackName,
            from: start,
            to: end,
          });
          return { name, rows };
        }),
      );

      return mergeTimeSeries(seriesPerEval);
    },
    staleTime: 60_000,
    retry: false,
  });
}

/**
 * Discover available eval metrics for the current workspace with current
 * values, types, and sparkline data.
 */
export function useEvalMetrics(filter?: EvalFilter) {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  return useQuery({
    queryKey: ["eval-metrics-discovery", workspace, filter],
    enabled: Boolean(workspace),
    queryFn: async (): Promise<EvalMetricInfo[]> => {
      if (!workspace) return [];

      const descriptors = await fetchEvalDescriptors(workspace);
      if (descriptors.length === 0) return [];

      const end = new Date();
      const start = new Date(end.getTime() - SPARKLINE_RANGE.seconds * 1000);

      const perMetric = await Promise.all(
        descriptors.map(async (desc) => {
          const [current, history] = await Promise.all([
            // Current value: collapse to a single number via groupBy=eval_id.
            fetchEvalAggregate({
              workspace,
              groupBy: "eval_id",
              metric: "avg_score",
              evalId: desc.evalId,
              agentName: filter?.agent,
              promptpackName: filter?.promptpackName,
              from: start,
              to: end,
            }),
            // Sparkline: time-bucketed series over the same window.
            fetchEvalAggregate({
              workspace,
              groupBy: SPARKLINE_RANGE.groupBy,
              metric: "avg_score",
              evalId: desc.evalId,
              agentName: filter?.agent,
              promptpackName: filter?.promptpackName,
              from: start,
              to: end,
            }),
          ]);
          return {
            name: desc.evalId,
            value: current[0]?.value ?? 0,
            metricType: classifyEvalType(desc.evalType),
            sparkline: history.map((p) => ({ value: p.value })),
          };
        }),
      );

      return perMetric;
    },
    staleTime: DEFAULT_STALE_TIME,
    retry: false,
  });
}

/**
 * Merge per-eval time-bucket rows into one chronologically-ordered series of
 * `{timestamp, values}` rows.
 *
 * Bucket keys arrive as either `YYYY-MM-DD` (day) or
 * `YYYY-MM-DDTHH:00:00Z` (hour) — both parse with `new Date(...)`.
 */
function mergeTimeSeries(
  perEval: { name: string; rows: { key: string; value: number }[] }[],
): EvalTrendPoint[] {
  const timeMap = new Map<number, Record<string, number>>();

  for (const { name, rows } of perEval) {
    const displayName = name.replace(/^omnia_eval_/, "");
    for (const row of rows) {
      const ts = new Date(row.key).getTime();
      if (!Number.isFinite(ts)) continue;
      let bucket = timeMap.get(ts);
      if (!bucket) {
        bucket = {};
        timeMap.set(ts, bucket);
      }
      bucket[displayName] = row.value;
    }
  }

  return Array.from(timeMap.entries())
    .sort(([a], [b]) => a - b)
    .map(([ts, values]) => ({
      timestamp: new Date(ts),
      values,
    }));
}

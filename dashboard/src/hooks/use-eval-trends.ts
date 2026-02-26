/**
 * Hook for fetching eval metric trends from Prometheus.
 *
 * Uses Prometheus range queries to get time-series data for eval metrics.
 * Eval metrics are dynamically named with "omnia_eval_" prefix and emitted
 * by the runtime MetricCollector.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useQuery } from "@tanstack/react-query";
import {
  queryPrometheusRange,
  queryPrometheus,
  type PrometheusMatrixResult,
  type PrometheusVectorResult,
} from "@/lib/prometheus";
import { EvalQueries } from "@/lib/prometheus-queries";

/** Time range options for trend queries. */
export const EVAL_TREND_RANGES = {
  "1h": { seconds: 3600, step: "1m" },
  "6h": { seconds: 21600, step: "5m" },
  "24h": { seconds: 86400, step: "15m" },
  "7d": { seconds: 604800, step: "1h" },
  "30d": { seconds: 2592000, step: "6h" },
} as const;

export type EvalTrendRange = keyof typeof EVAL_TREND_RANGES;

export interface EvalTrendPoint {
  timestamp: Date;
  values: Record<string, number>;
}

export interface EvalMetricInfo {
  name: string;
  value: number;
}

/**
 * Fetch eval pass rate trends from Prometheus as time-series data.
 *
 * Queries all omnia_eval_* metrics using avg_over_time for trend data.
 */
export function useEvalPassRateTrends(params?: {
  metricNames?: string[];
  timeRange?: EvalTrendRange;
}) {
  const timeRange = params?.timeRange ?? "24h";
  const metricNames = params?.metricNames;
  const rangeConfig = EVAL_TREND_RANGES[timeRange];

  return useQuery({
    queryKey: ["eval-trends", metricNames, timeRange],
    queryFn: async (): Promise<EvalTrendPoint[]> => {
      const end = new Date();
      const start = new Date(end.getTime() - rangeConfig.seconds * 1000);

      const names = metricNames ?? (await discoverEvalMetrics());
      if (names.length === 0) return [];

      const results = await Promise.all(
        names.map(async (name) => {
          const query = EvalQueries.metricAvgOverTime(name, rangeConfig.step);
          const resp = await queryPrometheusRange(query, start, end, rangeConfig.step);
          return { name, data: resp };
        })
      );

      return mergeTimeSeries(results);
    },
    staleTime: 60000,
    retry: false,
  });
}

/**
 * Discover available eval metrics from Prometheus.
 */
export function useEvalMetrics() {
  return useQuery({
    queryKey: ["eval-metrics-discovery"],
    queryFn: async (): Promise<EvalMetricInfo[]> => {
      const names = await discoverEvalMetrics();
      if (names.length === 0) return [];

      const results = await Promise.all(
        names.map(async (name) => {
          const resp = await queryPrometheus(name);
          const value =
            resp.status === "success" && resp.data?.result?.[0]?.value
              ? Number.parseFloat(resp.data.result[0].value[1]) || 0
              : 0;
          return { name, value };
        })
      );

      return results;
    },
    staleTime: 30000,
    retry: false,
  });
}

/** Discover eval metric names from Prometheus. */
async function discoverEvalMetrics(): Promise<string[]> {
  try {
    const query = EvalQueries.discoverMetrics();
    const resp = await queryPrometheus(query);
    if (resp.status !== "success" || !resp.data?.result) return [];

    const names = new Set<string>();
    for (const item of resp.data.result as PrometheusVectorResult[]) {
      const name = item.metric.__name__;
      if (name && !name.endsWith("_bucket") && !name.endsWith("_sum") && !name.endsWith("_count")) {
        names.add(name);
      }
    }
    return Array.from(names).sort((a, b) => a.localeCompare(b));
  } catch {
    return [];
  }
}

/** Merge multiple time series into a unified array of points. */
function mergeTimeSeries(
  results: { name: string; data: { status: string; data?: { result: PrometheusMatrixResult[] } } }[]
): EvalTrendPoint[] {
  const timeMap = new Map<number, Record<string, number>>();

  for (const { name, data } of results) {
    if (data.status !== "success" || !data.data?.result) continue;

    const displayName = name.replace(/^omnia_eval_/, "");
    for (const series of data.data.result) {
      for (const [timestamp, value] of series.values ?? []) {
        if (!timeMap.has(timestamp)) {
          timeMap.set(timestamp, {});
        }
        timeMap.get(timestamp)![displayName] = Number.parseFloat(value) || 0;
      }
    }
  }

  return Array.from(timeMap.entries())
    .sort(([a], [b]) => a - b)
    .map(([timestamp, values]) => ({
      timestamp: new Date(timestamp * 1000),
      values,
    }));
}

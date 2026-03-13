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
} from "@/lib/prometheus";
import { EvalQueries, type EvalFilter } from "@/lib/prometheus-queries";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";
import { discoverEvalMetrics } from "@/lib/eval-discovery";
import type { EvalMetricType } from "@/types/eval";

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
  metricType: EvalMetricType;
  sparkline?: Array<{ value: number }>;
}

/**
 * Fetch eval score trends from Prometheus as time-series data.
 *
 * Queries all omnia_eval_* metrics using avg_over_time for trend data.
 */
export function useEvalScoreTrends(params?: {
  metricNames?: string[];
  timeRange?: EvalTrendRange;
  filter?: EvalFilter;
}) {
  const timeRange = params?.timeRange ?? "24h";
  const metricNames = params?.metricNames;
  const filter = params?.filter;
  const rangeConfig = EVAL_TREND_RANGES[timeRange];

  return useQuery({
    queryKey: ["eval-trends", metricNames, timeRange, filter],
    queryFn: async (): Promise<EvalTrendPoint[]> => {
      const end = new Date();
      const start = new Date(end.getTime() - rangeConfig.seconds * 1000);

      let names: string[];
      if (metricNames) {
        names = metricNames;
      } else {
        const discovered = await discoverEvalMetrics(filter);
        names = discovered.map((m) => m.name);
      }
      if (names.length === 0) return [];

      const results = await Promise.all(
        names.map(async (name) => {
          const query = EvalQueries.metricAvgOverTime(name, rangeConfig.step, filter);
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

/** Sparkline range config — last 1h at 1m resolution. */
const SPARKLINE_RANGE = { seconds: 3600, step: "1m" };

/**
 * Discover available eval metrics with current values, types, and sparkline data.
 *
 * Types come from the discovery call (metadata-resolved), not a separate fetch.
 */
export function useEvalMetrics(filter?: EvalFilter) {
  return useQuery({
    queryKey: ["eval-metrics-discovery", filter],
    queryFn: async (): Promise<EvalMetricInfo[]> => {
      const discovered = await discoverEvalMetrics(filter);
      if (discovered.length === 0) return [];

      const end = new Date();
      const start = new Date(end.getTime() - SPARKLINE_RANGE.seconds * 1000);

      const perMetric = await Promise.all(
        discovered.map(async (metric) => {
          const [instant, range] = await Promise.all([
            queryPrometheus(EvalQueries.metricValue(metric.name, filter)),
            queryPrometheusRange(
              EvalQueries.metricAvgOverTime(metric.name, SPARKLINE_RANGE.step, filter),
              start, end, SPARKLINE_RANGE.step,
            ),
          ]);
          const value =
            instant.status === "success" && instant.data?.result?.[0]?.value
              ? Number.parseFloat(instant.data.result[0].value[1]) || 0
              : 0;
          const sparkline = extractSparkline(range);
          return {
            name: metric.name,
            value,
            metricType: metric.metricType,
            sparkline,
          };
        }),
      );

      return perMetric;
    },
    staleTime: DEFAULT_STALE_TIME,
    retry: false,
  });
}

/** Extract sparkline points from a Prometheus range query response. */
function extractSparkline(
  resp: { status: string; data?: { result: PrometheusMatrixResult[] } },
): Array<{ value: number }> {
  if (resp.status !== "success" || !resp.data?.result?.[0]?.values) return [];
  return resp.data.result[0].values.map(([, v]: [number, string]) => ({
    value: Number.parseFloat(v) || 0,
  }));
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

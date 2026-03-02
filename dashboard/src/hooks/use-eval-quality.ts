/**
 * Hooks for agent quality dashboard with eval pass rates.
 *
 * useEvalSummary reads from Prometheus (eval gauge metrics).
 * useRecentEvalFailures is a placeholder -- the session-api does not yet
 * have an eval-results endpoint, so it returns empty data.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useQuery } from "@tanstack/react-query";
import { queryPrometheus, queryPrometheusMetadata, type PrometheusVectorResult, type PrometheusMetricType } from "@/lib/prometheus";
import { EvalQueries, type EvalFilter } from "@/lib/prometheus-queries";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";
import type { EvalResult, EvalResultSummary, EvalMetricType } from "@/types/eval";

export interface EvalListParams {
  agentName?: string;
  evalId?: string;
  evalType?: string;
  passed?: boolean;
  limit?: number;
  offset?: number;
}

/**
 * Discover eval metric names from Prometheus.
 */
/** Suffixes for infrastructure/histogram sub-metrics to exclude from discovery. */
const EXCLUDED_SUFFIXES = ["_bucket", "_sum", "_count", "_total", "_created"];

function isInfrastructureSuffix(name: string): boolean {
  return EXCLUDED_SUFFIXES.some((s) => name.endsWith(s));
}

async function discoverEvalMetrics(filter?: EvalFilter): Promise<string[]> {
  try {
    const query = EvalQueries.discoverMetrics(filter);
    const resp = await queryPrometheus(query);
    if (resp.status !== "success" || !resp.data?.result) return [];

    const names = new Set<string>();
    for (const item of resp.data.result as PrometheusVectorResult[]) {
      const name = item.metric.__name__;
      if (name && !isInfrastructureSuffix(name)) {
        names.add(name);
      }
    }
    return Array.from(names).sort((a, b) => a.localeCompare(b));
  } catch {
    return [];
  }
}

/** Map Prometheus type to EvalMetricType. */
function toEvalMetricType(promType: PrometheusMetricType): EvalMetricType {
  if (promType === "counter") return "counter";
  if (promType === "histogram") return "histogram";
  return "gauge";
}

/** Build an EvalResultSummary with type-aware field population. */
function buildSummary(name: string, value: number, metricType: EvalMetricType): EvalResultSummary {
  const evalId = name.replace(/^omnia_eval_/, "");
  if (metricType === "counter") {
    return {
      evalId,
      evalType: "counter",
      passRate: 0,
      total: Math.round(value),
      passed: 0,
      failed: 0,
      metricType,
    };
  }
  if (metricType === "histogram") {
    return {
      evalId,
      evalType: "histogram",
      passRate: 0,
      total: 0,
      passed: 0,
      failed: 0,
      avgScore: value,
      metricType,
    };
  }
  // gauge / boolean — existing behavior
  return {
    evalId,
    evalType: "metric",
    passRate: value * 100,
    total: 0,
    passed: 0,
    failed: 0,
    avgScore: value,
    metricType,
  };
}

/**
 * Fetch eval summary (pass rates per eval) from Prometheus metrics.
 *
 * Discovers omnia_eval_* metrics, fetches their current values and types,
 * and transforms them into EvalResultSummary[] for the Overview tab.
 */
export function useEvalSummary(filter?: EvalFilter) {
  return useQuery({
    queryKey: ["eval-summary", filter],
    queryFn: async (): Promise<EvalResultSummary[]> => {
      const names = await discoverEvalMetrics(filter);
      if (names.length === 0) return [];

      const [metadata, ...valueResults] = await Promise.all([
        fetchMetricTypes(names),
        ...names.map(async (name) => {
          const query = EvalQueries.metricValue(name, filter);
          const resp = await queryPrometheus(query);
          const value =
            resp.status === "success" && resp.data?.result?.[0]?.value
              ? Number.parseFloat(resp.data.result[0].value[1]) || 0
              : 0;
          return { name, value };
        }),
      ]);

      const types = metadata as Record<string, EvalMetricType>;
      return valueResults.map((r) =>
        buildSummary(r.name, r.value, types[r.name] ?? "gauge"),
      );
    },
    staleTime: DEFAULT_STALE_TIME,
    retry: false,
  });
}

/** Fetch Prometheus type metadata for metric names. */
async function fetchMetricTypes(names: string[]): Promise<Record<string, EvalMetricType>> {
  try {
    const metadata = await queryPrometheusMetadata();
    const result: Record<string, EvalMetricType> = {};
    for (const name of names) {
      result[name] = toEvalMetricType(metadata[name] ?? "gauge");
    }
    return result;
  } catch {
    return {};
  }
}

/**
 * Placeholder for recent eval failures.
 *
 * The session-api does not yet expose an eval-results endpoint, so this
 * hook returns empty data. Once the Go backend adds
 * GET /api/v1/eval-results, this can be wired back up.
 */
export function useRecentEvalFailures(_params?: EvalListParams) {
  return useQuery({
    queryKey: ["eval-failures"],
    queryFn: async () => ({ evalResults: [] as EvalResult[], total: 0 }),
    staleTime: DEFAULT_STALE_TIME,
  });
}

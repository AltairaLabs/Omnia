/**
 * Shared eval metric discovery utilities.
 *
 * Single source of truth for discovering eval metrics from Prometheus.
 * Uses metadata (not suffix heuristics) to identify histogram sub-metrics.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import {
  queryPrometheus,
  queryPrometheusMetadata,
  type PrometheusVectorResult,
  type PrometheusMetricType,
} from "@/lib/prometheus";
import { EvalQueries, type EvalFilter } from "@/lib/prometheus-queries";
import type { EvalMetricType } from "@/types/eval";

/** Prefixes for infrastructure metrics that are not eval quality metrics. */
const INFRA_PREFIXES = ["omnia_eval_worker_"];

/** Infrastructure metric names that aggregate across evals (not per-eval quality metrics). */
const INFRA_METRIC_NAMES = new Set([
  "omnia_eval_executed_total",
  "omnia_eval_score",
  "omnia_eval_duration_seconds",
  "omnia_eval_passed_total",
  "omnia_eval_failed_total",
]);

/** Suffixes that Prometheus adds to histogram base metrics. */
const HISTOGRAM_SUFFIXES = ["_bucket", "_sum", "_count", "_created"];

/** A discovered eval metric with its resolved type. */
export interface DiscoveredMetric {
  name: string;
  metricType: EvalMetricType;
}

/**
 * Discover eval metrics from Prometheus and resolve their types in one pass.
 *
 * Filtering strategy (in order):
 * 1. Infra prefix/name exclusion — hard-coded operational metrics
 * 2. Metadata-driven histogram sub-metric exclusion — a name is excluded
 *    only if it isn't in metadata AND stripping a histogram suffix yields
 *    a name that IS in metadata as type "histogram". This avoids false
 *    positives on eval names that happen to end with _count, _sum, etc.
 */
export async function discoverEvalMetrics(filter?: EvalFilter): Promise<DiscoveredMetric[]> {
  try {
    const [queryResp, metadata] = await Promise.all([
      queryPrometheus(EvalQueries.discoverMetrics(filter)),
      queryPrometheusMetadata(),
    ]);

    if (queryResp.status !== "success" || !queryResp.data?.result) return [];

    // Collect unique names from the instant query
    const rawNames = new Set<string>();
    for (const item of queryResp.data.result as PrometheusVectorResult[]) {
      const name = item.metric.__name__;
      if (name) rawNames.add(name);
    }

    const results: DiscoveredMetric[] = [];
    for (const name of rawNames) {
      if (isInfraMetric(name)) continue;
      if (isHistogramSubMetric(name, metadata)) continue;
      results.push({
        name,
        metricType: toEvalMetricType(metadata[name] ?? "gauge"),
      });
    }

    return results.sort((a, b) => a.name.localeCompare(b.name));
  } catch {
    return [];
  }
}

/** Check if a metric is an infrastructure/operational metric. */
function isInfraMetric(name: string): boolean {
  if (INFRA_PREFIXES.some((p) => name.startsWith(p))) return true;
  return INFRA_METRIC_NAMES.has(name);
}

/**
 * Check if a metric is a histogram sub-metric using Prometheus metadata.
 *
 * A metric is a histogram sub-metric if:
 * - It does NOT have its own entry in metadata (sub-metrics don't), AND
 * - Stripping one of the histogram suffixes yields a name that IS in
 *   metadata with type "histogram"
 *
 * This is authoritative — no false positives on names like "word_count".
 */
function isHistogramSubMetric(
  name: string,
  metadata: Record<string, PrometheusMetricType>,
): boolean {
  // If it has its own metadata entry, it's a real metric (not a sub-metric)
  if (name in metadata) return false;

  for (const suffix of HISTOGRAM_SUFFIXES) {
    if (name.endsWith(suffix)) {
      const base = name.slice(0, -suffix.length);
      if (metadata[base] === "histogram") return true;
    }
  }
  return false;
}

/** Map Prometheus type to EvalMetricType. */
export function toEvalMetricType(promType: PrometheusMetricType): EvalMetricType {
  if (promType === "counter") return "counter";
  if (promType === "histogram") return "histogram";
  return "gauge";
}

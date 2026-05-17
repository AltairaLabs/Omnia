/**
 * Eval-results service: structured (session-api) read path that replaces
 * Prometheus for product-class dashboard views.
 *
 * See CLAUDE.md → Observability Boundaries for the operational/product split.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import type { EvalMetricType } from "@/types/eval";

/** Valid groupBy values mirror session-api's EvalAggregateGroupBy. */
export type EvalAggregateGroupBy =
  | "eval_id"
  | "eval_type"
  | "agent"
  | "time:hour"
  | "time:day";

/** Valid metric values mirror session-api's EvalAggregateMetric. */
export type EvalAggregateMetric =
  | "count"
  | "avg_score"
  | "p50_score"
  | "p95_score"
  | "avg_latency_ms"
  | "p95_latency_ms";

/** One row of an aggregate response. */
export interface EvalAggregateRow {
  key: string;
  value: number;
  count: number;
}

/** A discovered eval scenario. */
export interface EvalDescriptor {
  evalId: string;
  evalType: string;
}

export interface EvalAggregateParams {
  /** Workspace name. Pinned to `namespace` server-side. Required. */
  workspace: string;
  groupBy: EvalAggregateGroupBy;
  metric: EvalAggregateMetric;
  /** Optional filters. */
  agentName?: string;
  promptpackName?: string;
  evalId?: string;
  evalType?: string;
  /** RFC3339 timestamps. */
  from?: Date;
  to?: Date;
}

/**
 * Fetch aggregate eval metrics for a workspace.
 * Throws on non-2xx response so React Query can surface the failure.
 */
export async function fetchEvalAggregate(
  params: EvalAggregateParams,
  fetchImpl: typeof fetch = fetch,
): Promise<EvalAggregateRow[]> {
  const qs = new URLSearchParams();
  qs.set("groupBy", params.groupBy);
  qs.set("metric", params.metric);
  if (params.agentName) qs.set("agentName", params.agentName);
  if (params.promptpackName) qs.set("promptpackName", params.promptpackName);
  if (params.evalId) qs.set("evalId", params.evalId);
  if (params.evalType) qs.set("evalType", params.evalType);
  if (params.from) qs.set("from", params.from.toISOString());
  if (params.to) qs.set("to", params.to.toISOString());

  const url = `/api/workspaces/${encodeURIComponent(params.workspace)}/eval-results/aggregate?${qs}`;
  const resp = await fetchImpl(url, { headers: { Accept: "application/json" } });
  if (!resp.ok) {
    throw new Error(`eval-aggregate: ${resp.status} ${resp.statusText}`);
  }
  const body = (await resp.json()) as { rows?: EvalAggregateRow[] };
  return body.rows ?? [];
}

/**
 * Discovery payload returned by the workspace's /eval-results/discover
 * proxy. Mirrors session-api's `EvalDiscoveryResult` JSON shape.
 */
export interface EvalDiscoveryResult {
  evals: EvalDescriptor[];
  agents: string[];
  promptpacks: string[];
}

/**
 * Fetch the full discovery payload for a workspace — distinct evals plus
 * the agent_name / promptpack_name label values used by the filter UI.
 * Replaces Prometheus' metric-name + label-value discovery for product views.
 */
export async function fetchEvalDiscovery(
  workspace: string,
  fetchImpl: typeof fetch = fetch,
): Promise<EvalDiscoveryResult> {
  const url = `/api/workspaces/${encodeURIComponent(workspace)}/eval-results/discover`;
  const resp = await fetchImpl(url, { headers: { Accept: "application/json" } });
  if (!resp.ok) {
    throw new Error(`eval-discover: ${resp.status} ${resp.statusText}`);
  }
  const body = (await resp.json()) as Partial<EvalDiscoveryResult>;
  return {
    evals: body.evals ?? [],
    agents: body.agents ?? [],
    promptpacks: body.promptpacks ?? [],
  };
}

/**
 * Convenience wrapper for callers that only need the evals slice. Hits the
 * same endpoint as fetchEvalDiscovery — React Query will dedupe when both
 * are called with the same workspace key.
 */
export async function fetchEvalDescriptors(
  workspace: string,
  fetchImpl: typeof fetch = fetch,
): Promise<EvalDescriptor[]> {
  const { evals } = await fetchEvalDiscovery(workspace, fetchImpl);
  return evals;
}

/**
 * Map a session-api eval_type string onto the dashboard's EvalMetricType
 * taxonomy. The Prom path resolved metric type via Prometheus metadata; with
 * the structured read path we infer from the eval handler's `eval_type` label.
 *
 * Anything not explicitly boolean (assertion / regex / json_path / equality
 * style checks) renders as a continuous gauge — same Y axis [0,1] as before.
 */
export function classifyEvalType(evalType: string): EvalMetricType {
  switch (evalType) {
    case "assertion":
    case "regex":
    case "json_path":
      return "boolean";
    default:
      return "gauge";
  }
}

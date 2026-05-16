/**
 * Eval-results service: structured (session-api) read path that replaces
 * Prometheus for product-class dashboard views.
 *
 * See CLAUDE.md → Observability Boundaries for the operational/product split.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

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
 * Fetch the set of distinct (evalId, evalType) pairs for a workspace.
 * Replaces Prometheus' metric-name discovery for product views.
 */
export async function fetchEvalDescriptors(
  workspace: string,
  fetchImpl: typeof fetch = fetch,
): Promise<EvalDescriptor[]> {
  const url = `/api/workspaces/${encodeURIComponent(workspace)}/eval-results/discover`;
  const resp = await fetchImpl(url, { headers: { Accept: "application/json" } });
  if (!resp.ok) {
    throw new Error(`eval-discover: ${resp.status} ${resp.statusText}`);
  }
  const body = (await resp.json()) as { evals?: EvalDescriptor[] };
  return body.evals ?? [];
}

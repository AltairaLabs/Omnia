/**
 * Provider-calls service: structured (session-api) read path for cost,
 * token usage, and request rate that replaces Prometheus for the cost +
 * provider-metrics dashboard hooks.
 *
 * See CLAUDE.md → Observability Boundaries for the operational/product split.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

/** Valid groupBy values mirror session-api's ProviderCallAggregateGroupBy. */
export type ProviderCallAggregateGroupBy =
  | "provider"
  | "provider_name"
  | "model"
  | "agent"
  | "time:hour"
  | "time:day";

/** Valid metric values mirror session-api's ProviderCallAggregateMetric. */
export type ProviderCallAggregateMetric =
  | "count"
  | "sum_cost_usd"
  | "sum_input_tokens"
  | "sum_output_tokens"
  | "sum_cached_tokens"
  | "sum_tokens"
  | "avg_duration_ms"
  | "p95_duration_ms";

/** One row of an aggregate response. */
export interface ProviderCallAggregateRow {
  key: string;
  value: number;
  count: number;
}

/** Discovery payload — distinct providers + provider names + models. */
export interface ProviderCallDiscoveryResult {
  providers: string[];
  providerNames: string[];
  models: string[];
}

export interface ProviderCallAggregateParams {
  /** Workspace name. Pinned to `namespace` server-side. Required. */
  workspace: string;
  groupBy: ProviderCallAggregateGroupBy | ProviderCallAggregateGroupBy[];
  metric: ProviderCallAggregateMetric;
  /** Optional filters. */
  agentName?: string;
  /** Provider type (e.g. "openai"). */
  provider?: string;
  /** Provider CRD name — distinguishes same-type providers. */
  providerName?: string;
  model?: string;
  /** RFC3339 timestamps. */
  from?: Date;
  to?: Date;
}

/**
 * Fetch aggregate provider-call metrics for a workspace.
 *
 * Returns [] on any non-2xx response. Cost/usage is non-critical product data,
 * and the proxy legitimately returns non-2xx when a workspace has no session-api
 * configured or it is unreachable — callers render "no data" rather than
 * surfacing an error (the dashboard already treats this as a soft signal).
 */
export async function fetchProviderCallsAggregate(
  params: ProviderCallAggregateParams,
  fetchImpl: typeof fetch = fetch,
): Promise<ProviderCallAggregateRow[]> {
  const qs = new URLSearchParams();
  qs.set(
    "groupBy",
    Array.isArray(params.groupBy) ? params.groupBy.join(",") : params.groupBy,
  );
  qs.set("metric", params.metric);
  if (params.agentName) qs.set("agentName", params.agentName);
  if (params.provider) qs.set("provider", params.provider);
  if (params.providerName) qs.set("providerName", params.providerName);
  if (params.model) qs.set("model", params.model);
  if (params.from) qs.set("from", params.from.toISOString());
  if (params.to) qs.set("to", params.to.toISOString());

  const url = `/api/workspaces/${encodeURIComponent(params.workspace)}/provider-calls/aggregate?${qs}`;
  const resp = await fetchImpl(url, { headers: { Accept: "application/json" } });
  if (!resp.ok) return [];
  const body = (await resp.json().catch(() => null)) as { rows?: ProviderCallAggregateRow[] } | null;
  return body?.rows ?? [];
}

/**
 * Fetch the distinct (provider, provider_name, model) values for a workspace.
 * Replaces Prometheus label-value discovery for provider/model dropdowns.
 */
export async function fetchProviderCallsDiscovery(
  workspace: string,
  fetchImpl: typeof fetch = fetch,
): Promise<ProviderCallDiscoveryResult> {
  const url = `/api/workspaces/${encodeURIComponent(workspace)}/provider-calls/discover`;
  const resp = await fetchImpl(url, { headers: { Accept: "application/json" } });
  // Non-critical: empty discovery slices on any non-2xx (not-configured /
  // unreachable session-api) so provider/model dropdowns just show nothing.
  if (!resp.ok) return { providers: [], providerNames: [], models: [] };
  const body = (await resp.json().catch(() => null)) as Partial<ProviderCallDiscoveryResult> | null;
  return {
    providers: body?.providers ?? [],
    providerNames: body?.providerNames ?? [],
    models: body?.models ?? [],
  };
}

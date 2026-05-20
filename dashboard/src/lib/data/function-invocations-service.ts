/**
 * Function-invocations service: structured (session-api) read path for
 * the per-call audit rows persisted by function-mode AgentRuntimes
 * (Functions Phase 1, #1102 / #1103 PR 5).
 *
 * Mirrors the provider-calls service shape — functional module, no
 * class, fetch is injected so tests don't need to mock global fetch.
 *
 * See CLAUDE.md → Observability Boundaries: function invocations are
 * product data, not operational, so they live in session-api and not
 * Prometheus.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

/** FunctionInvocationStatus mirrors the CHECK constraint on the
 * function_invocations table. */
export type FunctionInvocationStatus =
  | "success"
  | "input_invalid"
  | "output_invalid"
  | "runtime_error";

/** FunctionInvocation is the per-call audit row returned by session-api.
 * Field names match the Go FunctionInvocation struct's json tags exactly
 * — changes there must flow through here. */
export interface FunctionInvocation {
  id: string;
  namespace: string;
  functionName: string;
  inputHash: string;
  /** Raw model output. Optional because runtime_error rows have no
   * output to persist. */
  outputJson?: unknown;
  status: FunctionInvocationStatus;
  durationMs: number;
  costUsd: number;
  /** W3C trace id of the OTel span that produced this invocation.
   * Empty when no span was bound to the request context. */
  traceId?: string;
  createdAt: string;
}

export interface FunctionInvocationListParams {
  /** Workspace name. Pinned to `namespace` server-side. Required. */
  workspace: string;
  /** Optional function-name filter — restricts to one Function within
   * the namespace. */
  functionName?: string;
  /** RFC3339 timestamps for the time-window filter. */
  from?: Date;
  to?: Date;
  /** Limit defaults to session-api's 100; capped at 1000. */
  limit?: number;
}

/** FunctionInvocationsListResponse matches the Go-side envelope
 * returned by GET /api/v1/function-invocations. */
interface FunctionInvocationsListResponse {
  rows?: FunctionInvocation[];
}

/**
 * Fetch recent function invocations for a workspace. Returns the rows
 * directly (drops the response envelope); throws on non-2xx so React
 * Query can surface the failure.
 */
export async function fetchFunctionInvocations(
  params: FunctionInvocationListParams,
  fetchImpl: typeof fetch = fetch,
): Promise<FunctionInvocation[]> {
  const qs = new URLSearchParams();
  if (params.functionName) qs.set("function", params.functionName);
  if (params.from) qs.set("from", params.from.toISOString());
  if (params.to) qs.set("to", params.to.toISOString());
  if (params.limit !== undefined) qs.set("limit", String(params.limit));

  const query = qs.toString();
  const suffix = query ? `?${query}` : "";
  const url = `/api/workspaces/${encodeURIComponent(params.workspace)}/function-invocations${suffix}`;
  const resp = await fetchImpl(url, { headers: { Accept: "application/json" } });
  if (!resp.ok) {
    throw new Error(`function-invocations-list: ${resp.status} ${resp.statusText}`);
  }
  const body = (await resp.json()) as FunctionInvocationsListResponse;
  return body.rows ?? [];
}

/**
 * Fetch one invocation by id, scoped to the workspace.
 * Throws on non-2xx (including 404) so the caller can distinguish
 * "missing" from "error" via the message.
 */
export async function fetchFunctionInvocation(
  workspace: string,
  id: string,
  fetchImpl: typeof fetch = fetch,
): Promise<FunctionInvocation> {
  const url = `/api/workspaces/${encodeURIComponent(workspace)}/function-invocations/${encodeURIComponent(id)}`;
  const resp = await fetchImpl(url, { headers: { Accept: "application/json" } });
  if (!resp.ok) {
    throw new Error(`function-invocation: ${resp.status} ${resp.statusText}`);
  }
  return (await resp.json()) as FunctionInvocation;
}

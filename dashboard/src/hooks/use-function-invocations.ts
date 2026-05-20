/**
 * Hook for fetching function-invocation audit rows from session-api.
 *
 * Consumes the workspace-scoped proxy added in #1103 PR 6 that fronts
 * GET /api/v1/function-invocations. The workspace name is pinned to
 * the namespace filter server-side so cross-tenant reads are impossible.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useQuery } from "@tanstack/react-query";
import {
  fetchFunctionInvocations,
  type FunctionInvocation,
  type FunctionInvocationListParams,
} from "@/lib/data/function-invocations-service";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";

export type UseFunctionInvocationsParams = Omit<FunctionInvocationListParams, "from" | "to"> & {
  /** Optional time window. When unset, session-api returns the most
   * recent rows up to `limit`. Passed through as ISO strings to
   * stabilise the query key (Date references churn on every render). */
  fromIso?: string;
  toIso?: string;
};

/**
 * Fetch invocation rows for a workspace, optionally scoped to one
 * function name + time window.
 *
 * The query key includes every filter so stale entries don't bleed
 * across views (e.g. switching from "summarizer" to "classifier"
 * should not show summarizer rows while the new query loads).
 */
export function useFunctionInvocations(params: UseFunctionInvocationsParams) {
  const { workspace, functionName, limit, fromIso, toIso } = params;
  return useQuery<FunctionInvocation[]>({
    queryKey: ["function-invocations", workspace, functionName ?? null, fromIso ?? null, toIso ?? null, limit ?? null],
    queryFn: () =>
      fetchFunctionInvocations({
        workspace,
        functionName,
        limit,
        from: fromIso ? new Date(fromIso) : undefined,
        to: toIso ? new Date(toIso) : undefined,
      }),
    staleTime: DEFAULT_STALE_TIME,
    enabled: Boolean(workspace),
  });
}

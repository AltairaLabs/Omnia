/**
 * Server-side fetch of a workspace's cost data from session-api.
 *
 * Reads exact aggregates from each service-group source and assembles them via
 * the pure cost-aggregation builder. Today the source list has one entry (the
 * "default" service group); the merge layer is built so additional service
 * groups can be added without changing assembly (see #1222 design, decision 5).
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { buildCostData, type CostAggregateInput } from "./cost-aggregation";
import type { CostData } from "./types";
import type { ProviderCallAggregateRow } from "./provider-calls-service";

/** One session-api source: a base URL and the namespace to scope queries to. */
export interface CostSource {
  sessionURL: string;
  namespace: string;
}

const EMPTY_ROWS: ProviderCallAggregateRow[] = [];

function trimSlash(u: string): string {
  return u.endsWith("/") ? u.slice(0, -1) : u;
}

async function fetchRows(
  source: CostSource,
  params: Record<string, string>,
  from: Date,
  to: Date,
  fetchImpl: typeof fetch,
): Promise<ProviderCallAggregateRow[]> {
  const qs = new URLSearchParams({
    ...params,
    namespace: source.namespace,
    from: from.toISOString(),
    to: to.toISOString(),
  });
  const url = `${trimSlash(source.sessionURL)}/api/v1/provider-calls/aggregate?${qs}`;
  const resp = await fetchImpl(url, { headers: { Accept: "application/json" } });
  if (!resp.ok) {
    throw new Error(`provider-calls aggregate: ${resp.status} ${resp.statusText}`);
  }
  const body = (await resp.json()) as { rows?: ProviderCallAggregateRow[] };
  return body.rows ?? EMPTY_ROWS;
}

const MATRIX_GROUP_BY = "provider,model,agent";
const SERIES_GROUP_BY = "time:hour,provider";

/**
 * Fetch + assemble CostData for a workspace across its service-group sources.
 * Sources that error are skipped; if every source fails, returns
 * available:false with a reason.
 */
export async function fetchWorkspaceCostData(
  sources: CostSource[],
  from: Date,
  to: Date,
  fetchImpl: typeof fetch = fetch,
): Promise<CostData> {
  const merged: CostAggregateInput = {
    cost: [],
    inputTokens: [],
    outputTokens: [],
    cachedTokens: [],
    requests: [],
    costByHourProvider: [],
    namespace: sources[0]?.namespace ?? "",
  };

  let anyOk = false;
  let lastError = "";

  for (const source of sources) {
    try {
      const [cost, inputTokens, outputTokens, cachedTokens, requests, series] =
        await Promise.all([
          fetchRows(source, { groupBy: MATRIX_GROUP_BY, metric: "sum_cost_usd" }, from, to, fetchImpl),
          fetchRows(source, { groupBy: MATRIX_GROUP_BY, metric: "sum_input_tokens" }, from, to, fetchImpl),
          fetchRows(source, { groupBy: MATRIX_GROUP_BY, metric: "sum_output_tokens" }, from, to, fetchImpl),
          fetchRows(source, { groupBy: MATRIX_GROUP_BY, metric: "sum_cached_tokens" }, from, to, fetchImpl),
          fetchRows(source, { groupBy: MATRIX_GROUP_BY, metric: "count" }, from, to, fetchImpl),
          fetchRows(source, { groupBy: SERIES_GROUP_BY, metric: "sum_cost_usd" }, from, to, fetchImpl),
        ]);
      merged.cost.push(...cost);
      merged.inputTokens.push(...inputTokens);
      merged.outputTokens.push(...outputTokens);
      merged.cachedTokens.push(...cachedTokens);
      merged.requests.push(...requests);
      merged.costByHourProvider.push(...series);
      anyOk = true;
    } catch (err) {
      lastError = err instanceof Error ? err.message : String(err);
    }
  }

  if (!anyOk) {
    return {
      available: false,
      reason: lastError || "Session API unavailable",
      summary: buildCostData(merged).summary,
      byAgent: [],
      byProvider: [],
      byModel: [],
      timeSeries: [],
    };
  }

  return buildCostData(merged);
}

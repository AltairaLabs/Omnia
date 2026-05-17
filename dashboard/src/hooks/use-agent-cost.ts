/**
 * Hook for fetching agent-specific cost metrics from session-api.
 *
 * Previously read Prometheus omnia_llm_* metrics; migrated to the
 * structured /api/workspaces/{name}/provider-calls/aggregate endpoint
 * as part of the observability split — see CLAUDE.md → Observability
 * Boundaries and
 * `docs/local-backlog/implemented/2026-04-17-observability-split-design.md`.
 *
 * Public surface (`AgentCostData`, `useAgentCost`) is unchanged so the
 * agent cards consume it without modification.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchProviderCallsAggregate } from "@/lib/data/provider-calls-service";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";

export interface AgentCostData {
  available: boolean;
  totalCost: number;
  inputTokens: number;
  outputTokens: number;
  requests: number;
  /** Time series for sparkline (last 24h, hourly resolution; 24 points). */
  timeSeries: Array<{ value: number }>;
}

const EMPTY_DATA: AgentCostData = {
  available: false,
  totalCost: 0,
  inputTokens: 0,
  outputTokens: 0,
  requests: 0,
  timeSeries: [],
};

const ONE_DAY_MS = 24 * 60 * 60 * 1000;
const ONE_HOUR_MS = 60 * 60 * 1000;

/**
 * Fetch cost data for a single agent.
 *
 * Five aggregate calls in parallel:
 *   - sum_cost_usd, sum_input_tokens, sum_output_tokens, count
 *     all grouped by agent (collapses to one row each)
 *   - sum_cost_usd grouped by time:hour over the last 24h (sparkline)
 */
async function fetchAgentCost(
  workspace: string,
  agentName: string,
): Promise<AgentCostData> {
  const end = new Date();
  const start = new Date(end.getTime() - ONE_DAY_MS);
  const base = {
    workspace,
    agentName,
    groupBy: "agent" as const,
    from: start,
    to: end,
  };

  try {
    const [costRows, inputRows, outputRows, requestRows, seriesRows] = await Promise.all([
      fetchProviderCallsAggregate({ ...base, metric: "sum_cost_usd" }),
      fetchProviderCallsAggregate({ ...base, metric: "sum_input_tokens" }),
      fetchProviderCallsAggregate({ ...base, metric: "sum_output_tokens" }),
      fetchProviderCallsAggregate({ ...base, metric: "count" }),
      fetchProviderCallsAggregate({
        workspace,
        agentName,
        groupBy: "time:hour",
        metric: "sum_cost_usd",
        from: start,
        to: end,
      }),
    ]);

    return {
      available: true,
      totalCost: costRows[0]?.value ?? 0,
      inputTokens: inputRows[0]?.value ?? 0,
      outputTokens: outputRows[0]?.value ?? 0,
      requests: requestRows[0]?.value ?? 0,
      timeSeries: buildSparkline(seriesRows, end),
    };
  } catch (error) {
    console.error(`Failed to fetch cost data for agent ${agentName}:`, error);
    return EMPTY_DATA;
  }
}

/**
 * Convert a `time:hour` aggregate result into a continuous 24-point hourly
 * sparkline ending at `endTime`. Buckets with no data render as 0 so the
 * sparkline is visually continuous.
 */
function buildSparkline(
  rows: Array<{ key: string; value: number }>,
  endTime: Date,
): Array<{ value: number }> {
  // Map bucket-start ISO string → value. Keys arrive as e.g.
  // "2026-05-01T13:00:00Z" from the time:hour group-by.
  const byBucket = new Map<number, number>();
  for (const row of rows) {
    const ts = new Date(row.key).getTime();
    if (Number.isFinite(ts)) byBucket.set(ts, row.value);
  }

  const currentHour = Math.floor(endTime.getTime() / ONE_HOUR_MS) * ONE_HOUR_MS;
  const points: Array<{ value: number }> = [];
  for (let i = 23; i >= 0; i--) {
    const bucketStart = currentHour - i * ONE_HOUR_MS;
    points.push({ value: byBucket.get(bucketStart) ?? 0 });
  }
  return points;
}

/**
 * Hook to fetch and cache agent cost data.
 *
 * @param workspace - Workspace name (was historically called `namespace`;
 *                    they're equivalent — workspace name doubles as the
 *                    Kubernetes namespace housing the agent's sessions).
 * @param agentName - Agent name
 */
export function useAgentCost(workspace: string, agentName: string) {
  return useQuery({
    queryKey: ["agent-cost", workspace, agentName],
    queryFn: () => fetchAgentCost(workspace, agentName),
    refetchInterval: 60000,
    staleTime: DEFAULT_STALE_TIME,
    enabled: Boolean(workspace && agentName),
  });
}

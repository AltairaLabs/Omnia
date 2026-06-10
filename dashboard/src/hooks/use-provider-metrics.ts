/**
 * Hook for fetching per-provider usage metrics from session-api.
 *
 * Previously read Prometheus omnia_llm_* metrics; migrated to the
 * structured /api/workspaces/{name}/provider-calls/aggregate endpoint
 * as part of the observability split — see CLAUDE.md → Observability
 * Boundaries and
 * `docs/local-backlog/implemented/2026-04-17-observability-split-design.md`.
 *
 * Public surface (`ProviderMetricsData`, `useProviderMetrics`) is unchanged
 * so the providers list, provider detail page, and topology summary cards
 * consume it without modification.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import { fetchProviderCallsAggregate } from "@/lib/data/provider-calls-service";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";

export interface ProviderMetricsData {
  /** Whether session-api returned usable data. */
  available: boolean;
  /** Time series data points for sparklines. */
  requestRate: Array<{ timestamp: Date; value: number }>;
  inputTokenRate: Array<{ timestamp: Date; value: number }>;
  outputTokenRate: Array<{ timestamp: Date; value: number }>;
  costRate: Array<{ timestamp: Date; value: number }>;
  /** Current/latest values (last point in each series). */
  currentRequestRate: number;
  currentInputTokenRate: number;
  currentOutputTokenRate: number;
  /** 24-hour totals. */
  totalCost24h: number;
  totalRequests24h: number;
  totalTokens24h: number;
}

const EMPTY_METRICS: ProviderMetricsData = {
  available: false,
  requestRate: [],
  inputTokenRate: [],
  outputTokenRate: [],
  costRate: [],
  currentRequestRate: 0,
  currentInputTokenRate: 0,
  currentOutputTokenRate: 0,
  totalCost24h: 0,
  totalRequests24h: 0,
  totalTokens24h: 0,
};

const ONE_DAY_MS = 24 * 60 * 60 * 1000;

/**
 * Convert aggregate rows into typed time-series points. Bucket keys are
 * ISO timestamps from a `time:hour` group-by — parse via Date.
 */
function rowsToSeries(
  rows: Array<{ key: string; value: number }>,
): Array<{ timestamp: Date; value: number }> {
  const points: Array<{ timestamp: Date; value: number }> = [];
  for (const row of rows) {
    const ts = new Date(row.key);
    if (Number.isNaN(ts.getTime())) continue;
    points.push({ timestamp: ts, value: row.value });
  }
  return points;
}

/**
 * Fetch usage metrics for a specific provider in the current workspace.
 *
 * Scoped by the Provider CRD name (`providerName`), NOT the provider type, so
 * two providers of the same type (e.g. two "openai" CRDs) report distinct
 * numbers instead of collapsing to one. `providerType` is retained for the
 * stable call-site signature but no longer drives the query.
 */
export function useProviderMetrics(providerName: string, _providerType?: string) {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;

  return useQuery({
    queryKey: ["provider-metrics", workspace, providerName],
    enabled: Boolean(workspace && providerName),
    refetchInterval: 60000,
    staleTime: DEFAULT_STALE_TIME,
    queryFn: async (): Promise<ProviderMetricsData> => {
      if (!workspace || !providerName) return EMPTY_METRICS;

      const end = new Date();
      const start = new Date(end.getTime() - ONE_DAY_MS);
      const base = {
        workspace,
        providerName,
        from: start,
        to: end,
      };

      try {
        const [
          countSeries,
          inputSeries,
          outputSeries,
          costSeries,
          totalRequests,
          totalInputTokens,
          totalOutputTokens,
          totalCost,
        ] = await Promise.all([
          // Sparkline series (one row per hour).
          fetchProviderCallsAggregate({ ...base, groupBy: "time:hour", metric: "count" }),
          fetchProviderCallsAggregate({ ...base, groupBy: "time:hour", metric: "sum_input_tokens" }),
          fetchProviderCallsAggregate({ ...base, groupBy: "time:hour", metric: "sum_output_tokens" }),
          fetchProviderCallsAggregate({ ...base, groupBy: "time:hour", metric: "sum_cost_usd" }),
          // 24h totals (one row collapsed by provider CRD name).
          fetchProviderCallsAggregate({ ...base, groupBy: "provider_name", metric: "count" }),
          fetchProviderCallsAggregate({ ...base, groupBy: "provider_name", metric: "sum_input_tokens" }),
          fetchProviderCallsAggregate({ ...base, groupBy: "provider_name", metric: "sum_output_tokens" }),
          fetchProviderCallsAggregate({ ...base, groupBy: "provider_name", metric: "sum_cost_usd" }),
        ]);

        const requestRate = rowsToSeries(countSeries);
        const inputTokenRate = rowsToSeries(inputSeries);
        const outputTokenRate = rowsToSeries(outputSeries);
        const costRate = rowsToSeries(costSeries);

        const totalRequests24h = totalRequests[0]?.value ?? 0;
        const totalInputTokens24h = totalInputTokens[0]?.value ?? 0;
        const totalOutputTokens24h = totalOutputTokens[0]?.value ?? 0;
        const totalCost24h = totalCost[0]?.value ?? 0;

        return {
          available: true,
          requestRate,
          inputTokenRate,
          outputTokenRate,
          costRate,
          currentRequestRate: requestRate[requestRate.length - 1]?.value ?? 0,
          currentInputTokenRate: inputTokenRate[inputTokenRate.length - 1]?.value ?? 0,
          currentOutputTokenRate: outputTokenRate[outputTokenRate.length - 1]?.value ?? 0,
          totalCost24h,
          totalRequests24h,
          totalTokens24h: totalInputTokens24h + totalOutputTokens24h,
        };
      } catch (error) {
        console.warn("Failed to fetch provider metrics:", error);
        return EMPTY_METRICS;
      }
    },
  });
}

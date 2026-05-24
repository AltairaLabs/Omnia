/**
 * Hook for memory-consolidation stats from Prometheus.
 *
 * Reads omnia_memory_consolidation_* metrics emitted by the
 * memory-api consolidation worker (see
 * internal/memory/consolidation/metrics.go). Aggregate stats only —
 * per-action drill-down lives in the memory-dashboard-ux sibling spec.
 *
 * Per CLAUDE.md observability boundary, this is operational signal
 * (worker state, action counts) so Prometheus is the source of truth.
 */

import { useQuery } from "@tanstack/react-query";
import { queryPrometheus } from "@/lib/prometheus";

export interface ConsolidationStats {
  passesTotal: number;
  actionsTotal: number;
  actionsByType: Record<string, number>;
}

const EMPTY_STATS: ConsolidationStats = {
  passesTotal: 0,
  actionsTotal: 0,
  actionsByType: {},
};

function parseScalar(
  response: { data?: { result?: { value?: [number, string] }[] } } | undefined,
): number {
  const raw = response?.data?.result?.[0]?.value?.[1];
  if (!raw) return 0;
  const n = Number(raw);
  return Number.isFinite(n) ? n : 0;
}

function parseByLabel(
  response:
    | { data?: { result?: { metric?: Record<string, string>; value?: [number, string] }[] } }
    | undefined,
  label: string,
): Record<string, number> {
  const out: Record<string, number> = {};
  for (const row of response?.data?.result ?? []) {
    const key = row.metric?.[label];
    if (!key) continue;
    const raw = row.value?.[1];
    const n = raw ? Number(raw) : 0;
    if (Number.isFinite(n)) out[key] = n;
  }
  return out;
}

/**
 * Returns the consolidation stats for the last `rangeDays` days.
 *
 * Empty when Prometheus is unreachable or no consolidation runs have
 * landed yet — the dashboard renders an empty-state in that case.
 */
export function useConsolidationStats(opts: { rangeDays: number }) {
  return useQuery({
    queryKey: ["consolidation-stats", opts.rangeDays],
    queryFn: async (): Promise<ConsolidationStats> => {
      const window = `[${opts.rangeDays}d]`;
      // Use allSettled so one Prometheus failure doesn't leave the other
      // two in-flight promises as unhandled rejections (Vitest catches
      // those and fails the test even when the queryFn returns cleanly).
      const [passes, actions, byType] = await Promise.allSettled([
        queryPrometheus(
          `sum(increase(omnia_memory_consolidation_passes_total${window}))`,
        ),
        queryPrometheus(
          `sum(increase(omnia_memory_consolidation_actions_total${window}))`,
        ),
        queryPrometheus(
          `sum by (action) (increase(omnia_memory_consolidation_actions_total${window}))`,
        ),
      ]);
      const passesValue = passes.status === "fulfilled" ? passes.value : undefined;
      const actionsValue = actions.status === "fulfilled" ? actions.value : undefined;
      const byTypeValue = byType.status === "fulfilled" ? byType.value : undefined;
      if (!passesValue && !actionsValue && !byTypeValue) {
        return EMPTY_STATS;
      }
      return {
        passesTotal: parseScalar(passesValue),
        actionsTotal: parseScalar(actionsValue),
        actionsByType: parseByLabel(byTypeValue, "action"),
      };
    },
  });
}

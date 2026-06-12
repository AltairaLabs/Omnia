import type { ProviderCall } from "@/types";

/**
 * Per-source token + cost totals, derived from a session's provider calls.
 * The session's headline totals count only the agent's own calls; this breakdown
 * surfaces the other participants (self-play user simulator, judges) that are
 * recorded separately so they don't inflate the agent's usage.
 */
export type TokenBreakdownRow = {
  source: string;
  label: string;
  inputTokens: number;
  outputTokens: number;
  costUsd: number;
  count: number;
};

const SOURCE_LABELS: Record<string, string> = {
  agent: "Agent",
  selfplay: "Self-play",
  judge: "Judge",
};

// Known sources render first, in this order; any others follow alphabetically.
const SOURCE_ORDER = ["agent", "selfplay", "judge"];

function sourceKey(source?: string): string {
  return source && source !== "" ? source : "agent";
}

/**
 * Groups completed provider calls by source and sums their tokens + cost.
 * Returns rows ordered agent → self-play → judge → (others alphabetically).
 */
export function providerCallsBySource(calls: ProviderCall[]): TokenBreakdownRow[] {
  const bySource = new Map<string, TokenBreakdownRow>();
  for (const pc of calls) {
    if (pc.status !== "completed") continue;
    const source = sourceKey(pc.source);
    const row =
      bySource.get(source) ??
      { source, label: SOURCE_LABELS[source] ?? source, inputTokens: 0, outputTokens: 0, costUsd: 0, count: 0 };
    row.inputTokens += pc.inputTokens ?? 0;
    row.outputTokens += pc.outputTokens ?? 0;
    row.costUsd += pc.costUsd ?? 0;
    row.count += 1;
    bySource.set(source, row);
  }

  return [...bySource.values()].sort((a, b) => {
    const ai = SOURCE_ORDER.indexOf(a.source);
    const bi = SOURCE_ORDER.indexOf(b.source);
    const ar = ai === -1 ? SOURCE_ORDER.length : ai;
    const br = bi === -1 ? SOURCE_ORDER.length : bi;
    return ar === br ? a.source.localeCompare(b.source) : ar - br;
  });
}

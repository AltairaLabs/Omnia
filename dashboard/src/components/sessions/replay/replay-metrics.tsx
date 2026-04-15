"use client";

import { useMemo } from "react";
import { metricsAt, type ReplaySource } from "@/lib/sessions/replay";

interface ReplayMetricsProps extends ReplaySource {
  readonly currentTimeMs: number;
}

function StatTile({ label, value, testId }: { label: string; value: string; testId: string }) {
  return (
    <div className="flex flex-col rounded-md border bg-background px-3 py-2 text-sm">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="font-mono font-semibold" data-testid={testId}>
        {value}
      </span>
    </div>
  );
}

export function ReplayMetrics({
  startedAt,
  currentTimeMs,
  messages,
  toolCalls,
  providerCalls,
}: ReplayMetricsProps) {
  const m = useMemo(
    () => metricsAt({ startedAt, messages, toolCalls, providerCalls }, currentTimeMs),
    [startedAt, messages, toolCalls, providerCalls, currentTimeMs],
  );
  return (
    <div className="grid grid-cols-2 gap-2 px-4 py-3 md:grid-cols-5">
      <StatTile label="Cost" value={`$${m.costUsd.toFixed(4)}`} testId="metric-cost" />
      <StatTile label="Tokens in" value={m.inputTokens.toLocaleString()} testId="metric-tokens-in" />
      <StatTile label="Tokens out" value={m.outputTokens.toLocaleString()} testId="metric-tokens-out" />
      <StatTile label="Messages" value={String(m.messageCount)} testId="metric-messages" />
      <StatTile label="Tool calls" value={String(m.toolCallCount)} testId="metric-tools" />
    </div>
  );
}

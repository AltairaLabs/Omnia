"use client";

import { useMemo } from "react";
import { metricsAt, type ReplaySource } from "@/lib/sessions/replay";

interface ReplayMetricsProps extends ReplaySource {
  readonly currentTimeMs: number;
}

function Stat({
  label,
  value,
  testId,
}: {
  readonly label: string;
  readonly value: string;
  readonly testId: string;
}) {
  return (
    <div className="flex items-baseline gap-1.5 whitespace-nowrap">
      <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
        {label}
      </span>
      <span className="font-mono font-medium" data-testid={testId}>
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
    <div className="flex flex-wrap items-center gap-x-5 gap-y-1 border-b bg-muted/20 px-4 py-1.5 text-xs">
      <Stat label="Cost" value={`$${m.costUsd.toFixed(4)}`} testId="metric-cost" />
      <Stat
        label="Tokens"
        value={`${m.inputTokens.toLocaleString()} / ${m.outputTokens.toLocaleString()}`}
        testId="metric-tokens-in"
      />
      <Stat label="Messages" value={String(m.messageCount)} testId="metric-messages" />
      <Stat label="Tools" value={String(m.toolCallCount)} testId="metric-tools" />
      <span className="sr-only" data-testid="metric-tokens-out">
        {m.outputTokens}
      </span>
    </div>
  );
}

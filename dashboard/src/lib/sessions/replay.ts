import type { Message, ToolCall, ProviderCall } from "@/types/session";

/** Elapsed ms from sessionStart to eventTs, clamped to 0. */
export function toElapsedMs(sessionStart: string, eventTs: string): number {
  const delta = new Date(eventTs).getTime() - new Date(sessionStart).getTime();
  return delta < 0 ? 0 : delta;
}

/** Span from session start to the latest event timestamp; 0 if none. */
export function sessionDurationMs(sessionStart: string, eventTimestamps: string[]): number {
  if (eventTimestamps.length === 0) return 0;
  const startMs = new Date(sessionStart).getTime();
  let latest = startMs;
  for (const t of eventTimestamps) {
    const ms = new Date(t).getTime();
    if (ms > latest) latest = ms;
  }
  return latest - startMs;
}

export interface ReplaySource {
  readonly startedAt: string;
  readonly messages?: readonly Message[];
  readonly toolCalls?: readonly ToolCall[];
  readonly providerCalls?: readonly ProviderCall[];
}

export interface ReplayMetrics {
  readonly costUsd: number;
  readonly inputTokens: number;
  readonly outputTokens: number;
  readonly messageCount: number;
  readonly toolCallCount: number;
  readonly providerCallCount: number;
}

/**
 * Aggregate metrics over events whose timestamp is <= currentTimeMs.
 *
 * Pending / failed provider calls contribute whatever usage fields they
 * have reported so far; any missing field (costUsd, inputTokens,
 * outputTokens) counts as 0. For completed sessions (v1 scope) every
 * call is terminal so this is exact; for live replay (future) the
 * running total will under-report a just-started call until its usage
 * lands.
 */
export function metricsAt(source: ReplaySource, currentTimeMs: number): ReplayMetrics {
  const { startedAt, messages = [], toolCalls = [], providerCalls = [] } = source;
  const cutoffMs = new Date(startedAt).getTime() + currentTimeMs;
  let costUsd = 0;
  let inputTokens = 0;
  let outputTokens = 0;
  let messageCount = 0;
  let toolCallCount = 0;
  let providerCallCount = 0;
  for (const m of messages) {
    if (new Date(m.timestamp).getTime() <= cutoffMs) messageCount++;
  }
  for (const tc of toolCalls) {
    if (new Date(tc.createdAt).getTime() <= cutoffMs) toolCallCount++;
  }
  for (const pc of providerCalls) {
    if (new Date(pc.createdAt).getTime() > cutoffMs) continue;
    providerCallCount++;
    costUsd += pc.costUsd ?? 0;
    inputTokens += pc.inputTokens ?? 0;
    outputTokens += pc.outputTokens ?? 0;
  }
  return { costUsd, inputTokens, outputTokens, messageCount, toolCallCount, providerCallCount };
}

export interface VisibleEvents {
  readonly messages: Message[];
  readonly toolCalls: ToolCall[];
}

/** Slice messages + tool calls to those visible at the given elapsed ms. */
export function visibleEventsAt(
  source: Pick<ReplaySource, "startedAt" | "messages" | "toolCalls">,
  currentTimeMs: number,
): VisibleEvents {
  const { startedAt, messages = [], toolCalls = [] } = source;
  const cutoffMs = new Date(startedAt).getTime() + currentTimeMs;
  return {
    messages: messages.filter((m) => new Date(m.timestamp).getTime() <= cutoffMs),
    toolCalls: toolCalls.filter((tc) => new Date(tc.createdAt).getTime() <= cutoffMs),
  };
}

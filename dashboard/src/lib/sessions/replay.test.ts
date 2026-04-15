import { describe, it, expect } from "vitest";
import { metricsAt, visibleEventsAt, sessionDurationMs, toElapsedMs } from "./replay";
import type { Message, ToolCall, ProviderCall } from "@/types/session";

const t0 = "2026-04-15T12:00:00.000Z";
const t500 = "2026-04-15T12:00:00.500Z";
const t1000 = "2026-04-15T12:00:01.000Z";
const t2000 = "2026-04-15T12:00:02.000Z";

const messages: Message[] = [
  { id: "m1", role: "user", content: "hi", timestamp: t0 },
  { id: "m2", role: "assistant", content: "hello", timestamp: t1000 },
];
const toolCalls: ToolCall[] = [
  { id: "tc1", callId: "c1", sessionId: "s", name: "search",
    arguments: {}, status: "success", createdAt: t500, durationMs: 100 },
];
const providerCalls: ProviderCall[] = [
  { id: "pc1", sessionId: "s", provider: "claude", model: "sonnet",
    status: "completed", inputTokens: 100, outputTokens: 50,
    costUsd: 0.01, createdAt: t500 },
  { id: "pc2", sessionId: "s", provider: "claude", model: "sonnet",
    status: "completed", inputTokens: 200, outputTokens: 80,
    costUsd: 0.03, createdAt: t2000 },
];

describe("toElapsedMs", () => {
  it("returns 0 at session start", () => {
    expect(toElapsedMs(t0, t0)).toBe(0);
  });
  it("returns positive ms for later timestamps", () => {
    expect(toElapsedMs(t0, t1000)).toBe(1000);
  });
  it("clamps negative (pre-session) to 0", () => {
    expect(toElapsedMs(t1000, t0)).toBe(0);
  });
});

describe("sessionDurationMs", () => {
  it("returns span from first to last event", () => {
    expect(sessionDurationMs(t0, [t0, t500, t2000])).toBe(2000);
  });
  it("returns 0 when there are no events", () => {
    expect(sessionDurationMs(t0, [])).toBe(0);
  });
});

describe("metricsAt", () => {
  it("at t=0 only counts events exactly at session start", () => {
    const m = metricsAt({ startedAt: t0, messages, toolCalls, providerCalls }, 0);
    expect(m).toEqual({
      costUsd: 0, inputTokens: 0, outputTokens: 0,
      messageCount: 1, toolCallCount: 0, providerCallCount: 0,
    });
  });
  it("includes only events whose timestamp <= currentTimeMs", () => {
    const m = metricsAt({ startedAt: t0, messages, toolCalls, providerCalls }, 600);
    expect(m.messageCount).toBe(1);
    expect(m.toolCallCount).toBe(1);
    expect(m.providerCallCount).toBe(1);
    expect(m.costUsd).toBeCloseTo(0.01, 5);
    expect(m.inputTokens).toBe(100);
    expect(m.outputTokens).toBe(50);
  });
  it("accumulates across the full session at t=end", () => {
    const m = metricsAt({ startedAt: t0, messages, toolCalls, providerCalls }, 2000);
    expect(m.messageCount).toBe(2);
    expect(m.providerCallCount).toBe(2);
    expect(m.costUsd).toBeCloseTo(0.04, 5);
  });
});

describe("visibleEventsAt", () => {
  it("returns messages + tool calls up to the given elapsed ms", () => {
    const v = visibleEventsAt({ startedAt: t0, messages, toolCalls }, 600);
    expect(v.messages.map((x) => x.id)).toEqual(["m1"]);
    expect(v.toolCalls.map((x) => x.id)).toEqual(["tc1"]);
  });
});

describe("metricsAt — missing usage fields", () => {
  it("treats missing provider-call usage fields as zero", () => {
    const pc: ProviderCall = {
      id: "pc-missing", sessionId: "s", provider: "claude", model: "sonnet",
      status: "pending", createdAt: t500,
      // no costUsd, inputTokens, outputTokens
    };
    const m = metricsAt({ startedAt: t0, providerCalls: [pc] }, 1000);
    expect(m.costUsd).toBe(0);
    expect(m.inputTokens).toBe(0);
    expect(m.outputTokens).toBe(0);
    expect(m.providerCallCount).toBe(1);
    expect(Number.isNaN(m.costUsd)).toBe(false);
  });
});

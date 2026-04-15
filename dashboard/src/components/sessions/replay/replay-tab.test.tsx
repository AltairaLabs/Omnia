import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { ReplayTab } from "./replay-tab";
import type { Session, Message, ToolCall, ProviderCall } from "@/types/session";

// Same rAF shim + fake timers as the playback hook test — the hook relies on rAF.
let rafId = 0;
const originalRaf = global.requestAnimationFrame;
const originalCancelRaf = global.cancelAnimationFrame;
beforeEach(() => {
  rafId = 0;
  global.requestAnimationFrame = (cb: FrameRequestCallback) => {
    rafId++;
    const id = rafId;
    setTimeout(() => cb(performance.now()), 16);
    return id;
  };
  global.cancelAnimationFrame = (id: number) => clearTimeout(id as unknown as NodeJS.Timeout);
});
afterEach(() => {
  global.requestAnimationFrame = originalRaf;
  global.cancelAnimationFrame = originalCancelRaf;
  vi.useRealTimers();
});

const t0 = "2026-04-15T12:00:00.000Z";
const session: Session = {
  id: "s1",
  agentName: "demo",
  agentNamespace: "test",
  status: "completed",
  startedAt: t0,
  endedAt: "2026-04-15T12:00:02.000Z",
  messages: [],
  metadata: {},
  metrics: { messageCount: 0, toolCallCount: 0, totalTokens: 0, inputTokens: 0, outputTokens: 0 },
};
const messages: Message[] = [
  { id: "m1", role: "user", content: "hi", timestamp: t0 },
];
const toolCalls: ToolCall[] = [];
const providerCalls: ProviderCall[] = [];

describe("ReplayTab", () => {
  it("renders controls, scrubber, metrics, conversation and event detail", () => {
    render(
      <ReplayTab
        session={session}
        messages={messages}
        toolCalls={toolCalls}
        providerCalls={providerCalls}
        runtimeEvents={[]}
      />
    );
    expect(screen.getByRole("button", { name: /play/i })).toBeInTheDocument();
    expect(screen.getByRole("slider")).toBeInTheDocument();
    expect(screen.getByTestId("metric-cost")).toBeInTheDocument();
  });

  it("renders a friendly empty state when the session has no events", () => {
    render(
      <ReplayTab
        session={session}
        messages={[]}
        toolCalls={[]}
        providerCalls={[]}
        runtimeEvents={[]}
      />
    );
    expect(screen.getByText(/nothing to replay/i)).toBeInTheDocument();
  });
});

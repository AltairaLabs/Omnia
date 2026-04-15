import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ReplayMetrics } from "./replay-metrics";
import type { Message, ProviderCall } from "@/types/session";

const t0 = "2026-04-15T12:00:00.000Z";
const t500 = "2026-04-15T12:00:00.500Z";
const messages: Message[] = [
  { id: "m1", role: "user", content: "hi", timestamp: t0 },
];
const providerCalls: ProviderCall[] = [
  { id: "pc1", sessionId: "s", provider: "claude", model: "sonnet",
    status: "completed", inputTokens: 100, outputTokens: 50,
    costUsd: 0.0123, createdAt: t500 },
];

describe("ReplayMetrics", () => {
  it("renders zeros before any non-start event", () => {
    render(
      <ReplayMetrics
        startedAt={t0}
        currentTimeMs={0}
        messages={messages}
        toolCalls={[]}
        providerCalls={providerCalls}
      />
    );
    expect(screen.getByTestId("metric-cost")).toHaveTextContent("$0.0000");
    expect(screen.getByTestId("metric-tokens-in")).toHaveTextContent("0");
  });

  it("updates as currentTimeMs advances past events", () => {
    render(
      <ReplayMetrics
        startedAt={t0}
        currentTimeMs={1000}
        messages={messages}
        toolCalls={[]}
        providerCalls={providerCalls}
      />
    );
    expect(screen.getByTestId("metric-cost")).toHaveTextContent("$0.0123");
    expect(screen.getByTestId("metric-tokens-in")).toHaveTextContent("100");
    expect(screen.getByTestId("metric-tokens-out")).toHaveTextContent("50");
    expect(screen.getByTestId("metric-messages")).toHaveTextContent("1");
  });
});

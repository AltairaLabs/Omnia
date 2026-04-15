import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ReplayConversation } from "./replay-conversation";
import type { Message, ToolCall } from "@/types/session";

const t0 = "2026-04-15T12:00:00.000Z";
const t500 = "2026-04-15T12:00:00.500Z";
const t1000 = "2026-04-15T12:00:01.000Z";

const messages: Message[] = [
  { id: "m1", role: "user", content: "hello", timestamp: t0 },
  { id: "m2", role: "assistant", content: "hi there", timestamp: t1000 },
];
const toolCalls: ToolCall[] = [
  { id: "tc1", callId: "c1", sessionId: "s", name: "search",
    arguments: { q: "cats" }, status: "success", createdAt: t500 },
];

describe("ReplayConversation", () => {
  it("renders only the t=0 message at currentTimeMs=0 (nothing later visible)", () => {
    render(
      <ReplayConversation
        startedAt={t0}
        currentTimeMs={0}
        messages={messages}
        toolCalls={toolCalls}
      />
    );
    expect(screen.getByText("hello")).toBeInTheDocument();
    expect(screen.queryByText("hi there")).not.toBeInTheDocument();
    expect(screen.queryByText(/search/)).not.toBeInTheDocument();
  });

  it("reveals events as currentTimeMs advances", () => {
    const { rerender } = render(
      <ReplayConversation
        startedAt={t0}
        currentTimeMs={600}
        messages={messages}
        toolCalls={toolCalls}
      />
    );
    expect(screen.getByText("hello")).toBeInTheDocument();
    expect(screen.getByText(/search/)).toBeInTheDocument();
    expect(screen.queryByText("hi there")).not.toBeInTheDocument();

    rerender(
      <ReplayConversation
        startedAt={t0}
        currentTimeMs={1500}
        messages={messages}
        toolCalls={toolCalls}
      />
    );
    expect(screen.getByText("hi there")).toBeInTheDocument();
  });
});

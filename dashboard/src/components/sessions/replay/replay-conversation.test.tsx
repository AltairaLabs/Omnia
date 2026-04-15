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
  {
    id: "tc-client",
    callId: "c1",
    sessionId: "s",
    name: "get_location",
    arguments: { precise: true },
    status: "success",
    result: { lat: 40.7, lon: -74.0 },
    createdAt: t500,
    labels: { handler_type: "client" },
  },
];

const serverCalls: ToolCall[] = [
  {
    id: "tc-server",
    callId: "c2",
    sessionId: "s",
    name: "search_web",
    arguments: { q: "cats" },
    status: "success",
    createdAt: t500,
    labels: { handler_type: "http" },
  },
];

describe("ReplayConversation", () => {
  it("renders only the t=0 message at currentTimeMs=0", () => {
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
    expect(screen.queryByText(/get_location/)).not.toBeInTheDocument();
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
    expect(screen.getByText(/get_location/)).toBeInTheDocument();
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

  it("labels a client tool call as 'client'", () => {
    render(
      <ReplayConversation
        startedAt={t0}
        currentTimeMs={2000}
        messages={[]}
        toolCalls={toolCalls}
      />
    );
    expect(screen.getByText("client")).toBeInTheDocument();
    expect(screen.queryByText("server")).not.toBeInTheDocument();
  });

  it("labels an http/openapi/grpc/mcp tool call as 'server'", () => {
    render(
      <ReplayConversation
        startedAt={t0}
        currentTimeMs={2000}
        messages={[]}
        toolCalls={serverCalls}
      />
    );
    expect(screen.getByText("server")).toBeInTheDocument();
    expect(screen.queryByText("client")).not.toBeInTheDocument();
  });

  it("labels a tool call with missing handler_type as 'unknown'", () => {
    const unknownTool: ToolCall[] = [
      {
        id: "tc-u",
        callId: "c3",
        sessionId: "s",
        name: "mystery",
        arguments: {},
        status: "success",
        createdAt: t500,
      },
    ];
    render(
      <ReplayConversation
        startedAt={t0}
        currentTimeMs={2000}
        messages={[]}
        toolCalls={unknownTool}
      />
    );
    expect(screen.getByText("unknown")).toBeInTheDocument();
  });

  it("renders error styling when a tool call failed", () => {
    const errored: ToolCall[] = [
      {
        id: "tc-err",
        callId: "c4",
        sessionId: "s",
        name: "broken",
        arguments: {},
        status: "error",
        errorMessage: "it broke",
        createdAt: t500,
        labels: { handler_type: "http" },
      },
    ];
    render(
      <ReplayConversation
        startedAt={t0}
        currentTimeMs={2000}
        messages={[]}
        toolCalls={errored}
      />
    );
    expect(screen.getByText("error")).toBeInTheDocument();
    expect(screen.getByText("it broke")).toBeInTheDocument();
  });
});

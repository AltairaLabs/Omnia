import { describe, it, expect } from "vitest";
import { extractTimelineEvents } from "./timeline";
import type { Message } from "@/types/session";

function makeMessage(overrides: Partial<Message> & { id: string }): Message {
  return {
    role: "user",
    content: "Hello",
    timestamp: "2024-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("extractTimelineEvents", () => {
  it("returns empty array for empty input", () => {
    expect(extractTimelineEvents([])).toEqual([]);
  });

  it("maps user messages to user_message kind", () => {
    const messages = [makeMessage({ id: "m1", role: "user", content: "Hi" })];
    const events = extractTimelineEvents(messages);

    expect(events).toHaveLength(1);
    expect(events[0].kind).toBe("user_message");
    expect(events[0].label).toBe("User message");
    expect(events[0].detail).toBe("Hi");
  });

  it("maps assistant messages to assistant_message kind", () => {
    const messages = [makeMessage({ id: "m1", role: "assistant", content: "Hello!" })];
    const events = extractTimelineEvents(messages);

    expect(events[0].kind).toBe("assistant_message");
    expect(events[0].label).toBe("Assistant response");
  });

  it("maps system messages to system_message kind", () => {
    const messages = [makeMessage({ id: "m1", role: "system", content: "System init" })];
    const events = extractTimelineEvents(messages);

    expect(events[0].kind).toBe("system_message");
    expect(events[0].label).toBe("System message");
  });

  it("skips tool role messages", () => {
    const messages = [
      makeMessage({ id: "m1", role: "user", content: "Q" }),
      makeMessage({ id: "m2", role: "tool", content: "result" }),
      makeMessage({ id: "m3", role: "assistant", content: "A" }),
    ];
    const events = extractTimelineEvents(messages);

    expect(events).toHaveLength(2);
    expect(events.every(e => e.kind !== "system_message" || e.id !== "m2")).toBe(true);
  });

  it("emits tool_call events from tool_call messages", () => {
    const messages: Message[] = [
      {
        id: "m1",
        role: "assistant",
        content: '{"name":"search","arguments":{"q":"test"}}',
        timestamp: "2024-01-01T00:00:01Z",
        metadata: { type: "tool_call", duration_ms: "250", status: "success" },
        toolCallId: "tc1",
      },
      {
        id: "m2",
        role: "assistant",
        content: '{"name":"fetch","arguments":{"url":"example.com"}}',
        timestamp: "2024-01-01T00:00:02Z",
        metadata: { type: "tool_call", status: "error" },
        toolCallId: "tc2",
      },
    ];
    const events = extractTimelineEvents(messages);

    expect(events).toHaveLength(2);
    expect(events[0].kind).toBe("tool_call");
    expect(events[0].label).toBe("Tool: search");
    expect(events[0].toolCallId).toBe("tc1");
    expect(events[0].duration).toBe(250);
    expect(events[0].status).toBe("success");
    expect(events[1].label).toBe("Tool: fetch");
    expect(events[1].status).toBe("error");
  });

  it("handles workflow transition events", () => {
    const messages = [
      makeMessage({
        id: "m1",
        role: "system",
        content: "Transitioning",
        metadata: { type: "workflow_transition", from: "idle", to: "running" },
      }),
    ];
    const events = extractTimelineEvents(messages);

    expect(events[0].kind).toBe("workflow_transition");
    expect(events[0].label).toBe("Workflow: idle → running");
    expect(events[0].metadata?.from).toBe("idle");
  });

  it("handles workflow transition without from/to metadata", () => {
    const messages = [
      makeMessage({
        id: "m1",
        role: "system",
        content: "Transitioning",
        metadata: { type: "workflow_transition" },
      }),
    ];
    const events = extractTimelineEvents(messages);

    expect(events[0].label).toBe("Workflow transition");
  });

  it("handles workflow completed events", () => {
    const messages = [
      makeMessage({
        id: "m1",
        role: "system",
        content: "Done",
        metadata: { type: "workflow_completed" },
      }),
    ];
    const events = extractTimelineEvents(messages);

    expect(events[0].kind).toBe("workflow_completed");
    expect(events[0].label).toBe("Workflow completed");
  });

  it("handles error events", () => {
    const messages = [
      makeMessage({
        id: "m1",
        role: "system",
        content: "Something failed",
        metadata: { type: "error" },
      }),
    ];
    const events = extractTimelineEvents(messages);

    expect(events[0].kind).toBe("error");
    expect(events[0].status).toBe("error");
  });

  it("sorts events chronologically", () => {
    const messages = [
      makeMessage({ id: "m3", role: "assistant", content: "C", timestamp: "2024-01-01T00:00:03Z" }),
      makeMessage({ id: "m1", role: "user", content: "A", timestamp: "2024-01-01T00:00:01Z" }),
      makeMessage({ id: "m2", role: "system", content: "B", timestamp: "2024-01-01T00:00:02Z" }),
    ];
    const events = extractTimelineEvents(messages);

    expect(events[0].id).toBe("m1");
    expect(events[1].id).toBe("m2");
    expect(events[2].id).toBe("m3");
  });

  it("truncates long content to 120 chars", () => {
    const longContent = "A".repeat(200);
    const messages = [makeMessage({ id: "m1", content: longContent })];
    const events = extractTimelineEvents(messages);

    expect(events[0].detail).toHaveLength(123); // 120 + "..."
    expect(events[0].detail!.endsWith("...")).toBe(true);
  });

  it("handles messages with empty content", () => {
    const messages = [makeMessage({ id: "m1", content: "" })];
    const events = extractTimelineEvents(messages);

    expect(events[0].detail).toBeUndefined();
  });

  it("handles pipeline.started events", () => {
    const messages = [
      makeMessage({
        id: "m1",
        role: "system",
        content: '{"MiddlewareCount":6}',
        metadata: { source: "runtime", type: "pipeline.started" },
      }),
    ];
    const events = extractTimelineEvents(messages);

    expect(events[0].kind).toBe("pipeline_event");
    expect(events[0].label).toBe("Pipeline started");
  });

  it("handles pipeline.completed events", () => {
    const messages = [
      makeMessage({
        id: "m1",
        role: "system",
        content: '{"Duration":30020821144}',
        metadata: { source: "runtime", type: "pipeline.completed" },
      }),
    ];
    const events = extractTimelineEvents(messages);

    expect(events[0].kind).toBe("pipeline_event");
    expect(events[0].label).toBe("Pipeline completed");
  });

  it("handles stage events with name from JSON content", () => {
    const messages = [
      makeMessage({
        id: "m1",
        role: "system",
        content: '{"Name":"context_builder","Index":0,"StageType":"accumulate","Duration":0,"Error":null}',
        metadata: { source: "runtime", type: "stage.started" },
      }),
    ];
    const events = extractTimelineEvents(messages);

    expect(events[0].kind).toBe("stage_event");
    expect(events[0].label).toBe("Stage: context_builder started");
  });

  it("handles stage.completed events", () => {
    const messages = [
      makeMessage({
        id: "m1",
        role: "system",
        content: '{"Name":"provider","Index":0,"StageType":"","Duration":30013473945,"Error":null}',
        metadata: { source: "runtime", type: "stage.completed" },
      }),
    ];
    const events = extractTimelineEvents(messages);

    expect(events[0].kind).toBe("stage_event");
    expect(events[0].label).toBe("Stage: provider completed");
  });

  it("handles stage events with invalid JSON content", () => {
    const messages = [
      makeMessage({
        id: "m1",
        role: "system",
        content: "not json",
        metadata: { source: "runtime", type: "stage.started" },
      }),
    ];
    const events = extractTimelineEvents(messages);

    expect(events[0].kind).toBe("stage_event");
    expect(events[0].label).toBe("Stage started");
  });

  it("handles provider_call events", () => {
    const messages = [
      makeMessage({
        id: "m1",
        role: "system",
        content: '{"cachedTokens":0,"cost":0,"durationMs":6542,"provider":"ollama"}',
        metadata: { source: "runtime", type: "provider_call" },
      }),
    ];
    const events = extractTimelineEvents(messages);

    expect(events[0].kind).toBe("provider_call");
    expect(events[0].label).toBe("Provider call");
  });

  it("handles mixed message types in chronological order", () => {
    const messages: Message[] = [
      { id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T00:00:00Z" },
      { id: "m2", role: "system", content: "Workflow started", timestamp: "2024-01-01T00:00:01Z", metadata: { type: "workflow_transition", from: "idle", to: "active" } },
      { id: "m3", role: "assistant", content: '{"name":"search","arguments":{}}', timestamp: "2024-01-01T00:00:02Z", metadata: { type: "tool_call", duration_ms: "100", status: "success" }, toolCallId: "tc1" },
      { id: "m4", role: "assistant", content: "Found results!", timestamp: "2024-01-01T00:00:03Z" },
    ];

    const events = extractTimelineEvents(messages);

    expect(events).toHaveLength(4);
    expect(events.map(e => e.kind)).toEqual([
      "user_message",
      "workflow_transition",
      "tool_call",
      "assistant_message",
    ]);
  });
});

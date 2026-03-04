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

  it("emits tool_call events from toolCalls array", () => {
    const messages: Message[] = [
      {
        id: "m1",
        role: "assistant",
        content: "Let me search",
        timestamp: "2024-01-01T00:00:01Z",
        toolCalls: [
          { id: "tc1", name: "search", arguments: { q: "test" }, status: "success", duration: 250 },
          { id: "tc2", name: "fetch", arguments: { url: "example.com" }, status: "error" },
        ],
      },
    ];
    const events = extractTimelineEvents(messages);

    // 1 assistant_message + 2 tool_call events
    expect(events).toHaveLength(3);

    const tcEvents = events.filter(e => e.kind === "tool_call");
    expect(tcEvents).toHaveLength(2);
    expect(tcEvents[0].label).toBe("search");
    expect(tcEvents[0].toolCallId).toBe("tc1");
    expect(tcEvents[0].duration).toBe(250);
    expect(tcEvents[0].status).toBe("success");
    expect(tcEvents[1].label).toBe("fetch");
    expect(tcEvents[1].status).toBe("error");
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

  it("handles mixed message types in chronological order", () => {
    const messages: Message[] = [
      { id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T00:00:00Z" },
      { id: "m2", role: "system", content: "Workflow started", timestamp: "2024-01-01T00:00:01Z", metadata: { type: "workflow_transition", from: "idle", to: "active" } },
      {
        id: "m3", role: "assistant", content: "Searching...", timestamp: "2024-01-01T00:00:02Z",
        toolCalls: [{ id: "tc1", name: "search", arguments: {}, status: "success", duration: 100 }],
      },
      { id: "m4", role: "assistant", content: "Found results!", timestamp: "2024-01-01T00:00:03Z" },
    ];

    const events = extractTimelineEvents(messages);

    expect(events).toHaveLength(5); // user + workflow + assistant + tool_call + assistant
    expect(events.map(e => e.kind)).toEqual([
      "user_message",
      "workflow_transition",
      "assistant_message",
      "tool_call",
      "assistant_message",
    ]);
  });
});

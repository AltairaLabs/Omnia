import { describe, it, expect } from "vitest";
import { extractTimelineEvents, toolCallsToTimelineEvents, providerCallsToTimelineEvents, runtimeEventsToTimelineEvents } from "./timeline";
import type { Message, ToolCall, ProviderCall, RuntimeEvent } from "@/types/session";

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

  it("maps tool role messages to tool_result kind", () => {
    const messages = [
      makeMessage({ id: "m1", role: "user", content: "Q" }),
      makeMessage({ id: "m2", role: "tool", content: '{"output":"42"}', metadata: { type: "tool_result", handler_name: "calc" } }),
      makeMessage({ id: "m3", role: "assistant", content: "A" }),
    ];
    const events = extractTimelineEvents(messages);

    expect(events).toHaveLength(3);
    expect(events[1].kind).toBe("tool_result");
    expect(events[1].label).toBe("Result: calc");
  });

  it("maps tool role without metadata to tool_result kind", () => {
    const messages = [
      makeMessage({ id: "m1", role: "tool", content: "result data" }),
    ];
    const events = extractTimelineEvents(messages);

    expect(events).toHaveLength(1);
    expect(events[0].kind).toBe("tool_result");
    expect(events[0].label).toBe("Tool result");
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

  it("merges first-class tool calls into timeline", () => {
    const messages: Message[] = [
      makeMessage({ id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T00:00:00Z" }),
      makeMessage({ id: "m2", role: "assistant", content: "Hello!", timestamp: "2024-01-01T00:00:03Z" }),
    ];
    const toolCalls: ToolCall[] = [
      {
        id: "tc1", callId: "call-1", sessionId: "s1", name: "search",
        arguments: { q: "test" }, status: "success", durationMs: 100,
        createdAt: "2024-01-01T00:00:01Z",
      },
    ];

    const events = extractTimelineEvents(messages, toolCalls);

    expect(events).toHaveLength(3);
    expect(events[1].kind).toBe("tool_call");
    expect(events[1].label).toBe("Tool: search");
    expect(events[1].status).toBe("success");
  });

  it("merges first-class provider calls into timeline", () => {
    const messages: Message[] = [
      makeMessage({ id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T00:00:00Z" }),
    ];
    const providerCalls: ProviderCall[] = [
      {
        id: "pc1", sessionId: "s1", provider: "claude", model: "sonnet",
        status: "completed", durationMs: 500, createdAt: "2024-01-01T00:00:01Z",
      },
    ];

    const events = extractTimelineEvents(messages, undefined, providerCalls);

    expect(events).toHaveLength(2);
    expect(events[1].kind).toBe("provider_call");
    expect(events[1].label).toBe("Provider: claude/sonnet");
    expect(events[1].status).toBe("success");
  });

  it("skips message-based tool events when first-class records provided", () => {
    const messages: Message[] = [
      makeMessage({ id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T00:00:00Z" }),
      {
        id: "m2", role: "assistant",
        content: '{"name":"search","arguments":{}}',
        timestamp: "2024-01-01T00:00:01Z",
        metadata: { type: "tool_call" }, toolCallId: "tc1",
      },
    ];
    const toolCalls: ToolCall[] = [
      {
        id: "tc1", callId: "call-1", sessionId: "s1", name: "search",
        arguments: {}, status: "success", createdAt: "2024-01-01T00:00:01Z",
      },
    ];

    const events = extractTimelineEvents(messages, toolCalls);

    // Should have user message + first-class tool call, not the message-based one
    expect(events).toHaveLength(2);
    expect(events[1].kind).toBe("tool_call");
    expect(events[1].id).toBe("tc-tc1");
  });
});

describe("toolCallsToTimelineEvents", () => {
  it("converts tool calls to timeline events", () => {
    const toolCalls: ToolCall[] = [
      {
        id: "tc1", callId: "call-1", sessionId: "s1", name: "search",
        arguments: { q: "test" }, status: "success", durationMs: 100,
        createdAt: "2024-01-01T00:00:01Z",
      },
      {
        id: "tc2", callId: "call-2", sessionId: "s1", name: "fetch",
        arguments: {}, status: "error", errorMessage: "timeout",
        createdAt: "2024-01-01T00:00:02Z",
      },
    ];

    const events = toolCallsToTimelineEvents(toolCalls);

    expect(events).toHaveLength(2);
    expect(events[0].kind).toBe("tool_call");
    expect(events[0].label).toBe("Tool: search");
    expect(events[0].status).toBe("success");
    expect(events[0].duration).toBe(100);
    expect(events[1].status).toBe("error");
  });
});

describe("providerCallsToTimelineEvents", () => {
  it("converts provider calls to timeline events", () => {
    const providerCalls: ProviderCall[] = [
      {
        id: "pc1", sessionId: "s1", provider: "claude", model: "sonnet",
        status: "completed", durationMs: 2000,
        createdAt: "2024-01-01T00:00:01Z",
      },
      {
        id: "pc2", sessionId: "s1", provider: "claude", model: "sonnet",
        status: "failed", errorMessage: "rate limited",
        createdAt: "2024-01-01T00:00:02Z",
      },
    ];

    const events = providerCallsToTimelineEvents(providerCalls);

    expect(events).toHaveLength(2);
    expect(events[0].kind).toBe("provider_call");
    expect(events[0].label).toBe("Provider: claude/sonnet");
    expect(events[0].status).toBe("success");
    expect(events[0].duration).toBe(2000);
    expect(events[1].status).toBe("error");
  });
});

describe("runtimeEventsToTimelineEvents", () => {
  it("converts pipeline events", () => {
    const events: RuntimeEvent[] = [
      {
        id: "re1", sessionId: "s1", eventType: "pipeline.started",
        data: { MiddlewareCount: 3 }, timestamp: "2024-01-01T00:00:00Z",
      },
      {
        id: "re2", sessionId: "s1", eventType: "pipeline.completed",
        data: { Duration: 5000000000 }, durationMs: 5000,
        timestamp: "2024-01-01T00:00:05Z",
      },
    ];

    const result = runtimeEventsToTimelineEvents(events);

    expect(result).toHaveLength(2);
    expect(result[0].kind).toBe("pipeline_event");
    expect(result[0].label).toBe("Pipeline started");
    expect(result[1].kind).toBe("pipeline_event");
    expect(result[1].label).toBe("Pipeline completed");
    expect(result[1].status).toBe("success");
    expect(result[1].duration).toBe(5000);
  });

  it("converts stage events with names", () => {
    const events: RuntimeEvent[] = [
      {
        id: "re1", sessionId: "s1", eventType: "stage.started",
        data: { Name: "context_builder" }, timestamp: "2024-01-01T00:00:00Z",
      },
      {
        id: "re2", sessionId: "s1", eventType: "stage.completed",
        data: { Name: "generate" }, durationMs: 2000,
        timestamp: "2024-01-01T00:00:02Z",
      },
    ];

    const result = runtimeEventsToTimelineEvents(events);

    expect(result).toHaveLength(2);
    expect(result[0].kind).toBe("stage_event");
    expect(result[0].label).toBe("Stage: context_builder started");
    expect(result[1].label).toBe("Stage: generate completed");
    expect(result[1].status).toBe("success");
  });

  it("converts failed events with error status", () => {
    const events: RuntimeEvent[] = [
      {
        id: "re1", sessionId: "s1", eventType: "pipeline.failed",
        errorMessage: "context deadline exceeded",
        timestamp: "2024-01-01T00:00:00Z",
      },
      {
        id: "re2", sessionId: "s1", eventType: "validation.failed",
        data: { ValidatorName: "safety" }, errorMessage: "harmful content",
        timestamp: "2024-01-01T00:00:01Z",
      },
    ];

    const result = runtimeEventsToTimelineEvents(events);

    expect(result).toHaveLength(2);
    expect(result[0].status).toBe("error");
    expect(result[0].label).toBe("Pipeline failed");
    expect(result[0].detail).toBe("context deadline exceeded");
    expect(result[1].status).toBe("error");
  });

  it("converts workflow events", () => {
    const events: RuntimeEvent[] = [
      {
        id: "re1", sessionId: "s1", eventType: "workflow.transitioned",
        data: { FromState: "greeting", ToState: "info" },
        timestamp: "2024-01-01T00:00:00Z",
      },
      {
        id: "re2", sessionId: "s1", eventType: "workflow.completed",
        timestamp: "2024-01-01T00:00:05Z",
      },
    ];

    const result = runtimeEventsToTimelineEvents(events);

    expect(result).toHaveLength(2);
    expect(result[0].kind).toBe("workflow_transition");
    expect(result[1].kind).toBe("workflow_completed");
  });

  it("converts middleware and validation events", () => {
    const events: RuntimeEvent[] = [
      {
        id: "re1", sessionId: "s1", eventType: "middleware.started",
        data: { Name: "auth" }, timestamp: "2024-01-01T00:00:00Z",
      },
      {
        id: "re2", sessionId: "s1", eventType: "validation.passed",
        data: { ValidatorName: "safety" }, timestamp: "2024-01-01T00:00:01Z",
      },
    ];

    const result = runtimeEventsToTimelineEvents(events);

    expect(result).toHaveLength(2);
    expect(result[0].kind).toBe("stage_event");
    expect(result[0].label).toBe("Middleware: auth started");
    expect(result[1].kind).toBe("stage_event");
    expect(result[1].status).toBe("success");
  });

  it("converts context/state events", () => {
    const events: RuntimeEvent[] = [
      { id: "re1", sessionId: "s1", eventType: "context_built", timestamp: "2024-01-01T00:00:00Z" },
      { id: "re2", sessionId: "s1", eventType: "stream_interrupted", timestamp: "2024-01-01T00:00:01Z" },
      { id: "re3", sessionId: "s1", eventType: "state_loaded", timestamp: "2024-01-01T00:00:02Z" },
      { id: "re4", sessionId: "s1", eventType: "token_budget_exceeded", timestamp: "2024-01-01T00:00:03Z" },
    ];

    const result = runtimeEventsToTimelineEvents(events);

    expect(result).toHaveLength(4);
    expect(result[0].label).toBe("Context built");
    expect(result[1].label).toBe("Stream interrupted");
    expect(result[1].kind).toBe("error");
    expect(result[2].label).toBe("State loaded");
    expect(result[3].label).toBe("Token budget exceeded");
  });

  it("handles unknown event types as system_message", () => {
    const events: RuntimeEvent[] = [
      { id: "re1", sessionId: "s1", eventType: "custom.event", timestamp: "2024-01-01T00:00:00Z" },
    ];

    const result = runtimeEventsToTimelineEvents(events);

    expect(result).toHaveLength(1);
    expect(result[0].kind).toBe("system_message");
    expect(result[0].label).toBe("custom.event");
  });

  it("converts eval events with explanation", () => {
    const events: RuntimeEvent[] = [
      {
        id: "re1", sessionId: "s1", eventType: "eval.completed",
        data: { eval_id: "accuracy", passed: true, explanation: "Score 0.9 above threshold" },
        durationMs: 500, timestamp: "2024-01-01T00:00:00Z",
      },
      {
        id: "re2", sessionId: "s1", eventType: "eval.failed",
        data: { eval_id: "safety", passed: false, explanation: "Harmful content detected" },
        errorMessage: "safety check failed",
        timestamp: "2024-01-01T00:00:01Z",
      },
    ];

    const result = runtimeEventsToTimelineEvents(events);

    expect(result).toHaveLength(2);
    expect(result[0].kind).toBe("eval_event");
    expect(result[0].label).toBe("Eval: accuracy (passed)");
    expect(result[0].status).toBe("success");
    expect(result[1].kind).toBe("eval_event");
    expect(result[1].label).toBe("Eval: safety (failed)");
    expect(result[1].status).toBe("error");
    expect(result[1].detail).toBe("safety check failed");
  });
});

describe("extractTimelineEvents with runtimeEvents", () => {
  it("merges runtime events into timeline", () => {
    const messages: Message[] = [
      makeMessage({ id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T00:00:00Z" }),
      makeMessage({ id: "m2", role: "assistant", content: "Hello!", timestamp: "2024-01-01T00:00:05Z" }),
    ];
    const runtimeEvents: RuntimeEvent[] = [
      {
        id: "re1", sessionId: "s1", eventType: "pipeline.started",
        timestamp: "2024-01-01T00:00:01Z",
      },
      {
        id: "re2", sessionId: "s1", eventType: "pipeline.completed",
        durationMs: 3000, timestamp: "2024-01-01T00:00:04Z",
      },
    ];

    const events = extractTimelineEvents(messages, undefined, undefined, runtimeEvents);

    expect(events).toHaveLength(4);
    expect(events[0].kind).toBe("user_message");
    expect(events[1].kind).toBe("pipeline_event");
    expect(events[2].kind).toBe("pipeline_event");
    expect(events[3].kind).toBe("assistant_message");
  });

  it("skips message-based pipeline events when runtime events exist", () => {
    const messages: Message[] = [
      makeMessage({ id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T00:00:00Z" }),
      makeMessage({
        id: "m2", role: "system",
        content: '{"MiddlewareCount":3}',
        timestamp: "2024-01-01T00:00:01Z",
        metadata: { source: "runtime", type: "pipeline.started" },
      }),
    ];
    const runtimeEvents: RuntimeEvent[] = [
      {
        id: "re1", sessionId: "s1", eventType: "pipeline.started",
        data: { MiddlewareCount: 3 }, timestamp: "2024-01-01T00:00:01Z",
      },
    ];

    const events = extractTimelineEvents(messages, undefined, undefined, runtimeEvents);

    // User message + first-class runtime event only. Message-based pipeline event is skipped.
    expect(events).toHaveLength(2);
    expect(events[0].kind).toBe("user_message");
    expect(events[1].id).toBe("re-re1");
  });

  it("skips message-based system messages when runtime events exist", () => {
    const messages: Message[] = [
      makeMessage({ id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T00:00:00Z" }),
      makeMessage({
        id: "m2", role: "system",
        content: '{"Name":"context_builder"}',
        timestamp: "2024-01-01T00:00:01Z",
        metadata: { source: "runtime", type: "stage.started" },
      }),
      makeMessage({
        id: "m3", role: "system",
        content: "generic system event",
        timestamp: "2024-01-01T00:00:02Z",
      }),
    ];
    const runtimeEvents: RuntimeEvent[] = [
      {
        id: "re1", sessionId: "s1", eventType: "stage.started",
        data: { Name: "context_builder" }, timestamp: "2024-01-01T00:00:01Z",
      },
    ];

    const events = extractTimelineEvents(messages, undefined, undefined, runtimeEvents);

    // Only user message + runtime event. Both system messages skipped.
    expect(events).toHaveLength(2);
    expect(events[0].kind).toBe("user_message");
    expect(events[1].id).toBe("re-re1");
  });

  it("merges all first-class record types together", () => {
    const messages: Message[] = [
      makeMessage({ id: "m1", role: "user", content: "Hi", timestamp: "2024-01-01T00:00:00Z" }),
    ];
    const toolCalls: ToolCall[] = [
      {
        id: "tc1", callId: "call-1", sessionId: "s1", name: "search",
        arguments: {}, status: "success", createdAt: "2024-01-01T00:00:02Z",
      },
    ];
    const providerCalls: ProviderCall[] = [
      {
        id: "pc1", sessionId: "s1", provider: "claude", model: "sonnet",
        status: "completed", createdAt: "2024-01-01T00:00:03Z",
      },
    ];
    const runtimeEvents: RuntimeEvent[] = [
      {
        id: "re1", sessionId: "s1", eventType: "pipeline.started",
        timestamp: "2024-01-01T00:00:01Z",
      },
    ];

    const events = extractTimelineEvents(messages, toolCalls, providerCalls, runtimeEvents);

    expect(events).toHaveLength(4);
    expect(events.map(e => e.kind)).toEqual([
      "user_message",
      "pipeline_event",
      "tool_call",
      "provider_call",
    ]);
  });
});

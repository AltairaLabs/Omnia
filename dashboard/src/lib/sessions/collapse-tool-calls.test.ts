import { describe, it, expect } from "vitest";
import { collapseToolCalls } from "./collapse-tool-calls";
import type { ToolCall } from "@/types/session";

function makeTc(overrides: Partial<ToolCall> & Pick<ToolCall, "id" | "callId" | "status" | "createdAt">): ToolCall {
  return {
    sessionId: "s1",
    name: "get_weather",
    arguments: {},
    ...overrides,
  };
}

describe("collapseToolCalls", () => {
  it("returns empty array for empty input", () => {
    expect(collapseToolCalls([])).toEqual([]);
  });

  it("passes through single events unchanged", () => {
    const tc = makeTc({ id: "1", callId: "c1", status: "success", createdAt: "2026-01-01T00:00:01Z" });
    expect(collapseToolCalls([tc])).toEqual([tc]);
  });

  it("collapses pending + success into success", () => {
    const pending = makeTc({
      id: "1", callId: "c1", status: "pending",
      arguments: { location: "NYC" },
      labels: { handler_type: "http" },
      createdAt: "2026-01-01T00:00:01Z",
    });
    const success = makeTc({
      id: "2", callId: "c1", status: "success",
      result: "72F",
      durationMs: 320,
      createdAt: "2026-01-01T00:00:02Z",
    });

    const result = collapseToolCalls([pending, success]);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("success");
    expect(result[0].result).toBe("72F");
    expect(result[0].durationMs).toBe(320);
    // Arguments and labels come from the started event
    expect(result[0].arguments).toEqual({ location: "NYC" });
    expect(result[0].labels).toEqual({ handler_type: "http" });
    // Timestamp is the started event's
    expect(result[0].createdAt).toBe("2026-01-01T00:00:01Z");
  });

  it("collapses pending + error into error", () => {
    const pending = makeTc({
      id: "1", callId: "c1", status: "pending",
      arguments: { q: "test" },
      createdAt: "2026-01-01T00:00:01Z",
    });
    const errored = makeTc({
      id: "2", callId: "c1", status: "error",
      errorMessage: "timeout",
      durationMs: 5000,
      createdAt: "2026-01-01T00:00:06Z",
    });

    const result = collapseToolCalls([pending, errored]);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("error");
    expect(result[0].errorMessage).toBe("timeout");
    expect(result[0].arguments).toEqual({ q: "test" });
  });

  it("handles multiple independent tool calls", () => {
    const events = [
      makeTc({ id: "1", callId: "c1", status: "pending", createdAt: "2026-01-01T00:00:01Z" }),
      makeTc({ id: "2", callId: "c2", status: "pending", createdAt: "2026-01-01T00:00:02Z" }),
      makeTc({ id: "3", callId: "c1", status: "success", createdAt: "2026-01-01T00:00:03Z" }),
      makeTc({ id: "4", callId: "c2", status: "success", createdAt: "2026-01-01T00:00:04Z" }),
    ];

    const result = collapseToolCalls(events);
    expect(result).toHaveLength(2);
    expect(result[0].callId).toBe("c1");
    expect(result[0].status).toBe("success");
    expect(result[1].callId).toBe("c2");
    expect(result[1].status).toBe("success");
  });

  it("keeps pending tool call when there is no resolution", () => {
    const pending = makeTc({
      id: "1", callId: "c1", status: "pending",
      createdAt: "2026-01-01T00:00:01Z",
    });

    const result = collapseToolCalls([pending]);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("pending");
  });

  it("keeps sequential tool calls that share a repeating callId distinct", () => {
    // Providers that reset their per-round call indexer (e.g. Gemini's
    // `call_0`, `call_0`, `call_0` across three sequential rounds) emit
    // the same callId for every distinct invocation. FIFO pairing matches
    // each pending with the next chronologically-following resolution of
    // the same callId, so all three calls appear in the timeline instead
    // of collapsing into one row.
    const events = [
      makeTc({
        id: "p1", callId: "call_0", status: "pending",
        arguments: { content: "first" },
        createdAt: "2026-01-01T00:00:01Z",
      }),
      makeTc({
        id: "s1", callId: "call_0", status: "success",
        result: '{"id":"m1"}',
        durationMs: 100,
        createdAt: "2026-01-01T00:00:02Z",
      }),
      makeTc({
        id: "p2", callId: "call_0", status: "pending",
        arguments: { content: "second" },
        createdAt: "2026-01-01T00:00:10Z",
      }),
      makeTc({
        id: "s2", callId: "call_0", status: "success",
        result: '{"id":"m2"}',
        durationMs: 50,
        createdAt: "2026-01-01T00:00:11Z",
      }),
      makeTc({
        id: "p3", callId: "call_0", status: "pending",
        arguments: { content: "third" },
        createdAt: "2026-01-01T00:00:20Z",
      }),
      makeTc({
        id: "s3", callId: "call_0", status: "success",
        result: '{"id":"m3"}',
        durationMs: 75,
        createdAt: "2026-01-01T00:00:21Z",
      }),
    ];

    const result = collapseToolCalls(events);
    expect(result).toHaveLength(3);
    expect(result.map((tc) => tc.arguments)).toEqual([
      { content: "first" },
      { content: "second" },
      { content: "third" },
    ]);
    expect(result.map((tc) => tc.result)).toEqual([
      '{"id":"m1"}',
      '{"id":"m2"}',
      '{"id":"m3"}',
    ]);
    expect(result.every((tc) => tc.status === "success")).toBe(true);
  });

  it("emits an orphan resolution as its own row when no matching pending exists", () => {
    const orphan = makeTc({
      id: "1", callId: "c1", status: "success",
      result: "ok",
      createdAt: "2026-01-01T00:00:01Z",
    });
    const result = collapseToolCalls([orphan]);
    expect(result).toHaveLength(1);
    expect(result[0].status).toBe("success");
  });

  it("sorts output by started timestamp", () => {
    const events = [
      makeTc({ id: "4", callId: "c2", status: "success", createdAt: "2026-01-01T00:00:04Z" }),
      makeTc({ id: "1", callId: "c1", status: "pending", createdAt: "2026-01-01T00:00:01Z" }),
      makeTc({ id: "2", callId: "c2", status: "pending", createdAt: "2026-01-01T00:00:02Z" }),
      makeTc({ id: "3", callId: "c1", status: "success", createdAt: "2026-01-01T00:00:03Z" }),
    ];

    const result = collapseToolCalls(events);
    expect(result).toHaveLength(2);
    expect(result[0].callId).toBe("c1");
    expect(result[1].callId).toBe("c2");
  });
});

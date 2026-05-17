/**
 * Tests for eval-results-service: the thin fetch wrapper used by
 * useEvalScoreTrends / useEvalMetrics after the observability split.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  classifyEvalType,
  fetchEvalAggregate,
  fetchEvalDescriptors,
  fetchEvalDiscovery,
} from "./eval-results-service";

beforeEach(() => {
  vi.restoreAllMocks();
});

describe("fetchEvalAggregate", () => {
  it("builds the workspace-scoped URL with required + optional params", async () => {
    const calls: Array<{ url: string }> = [];
    const fakeFetch = vi.fn(async (url: string) => {
      calls.push({ url });
      return new Response(
        JSON.stringify({
          rows: [
            { key: "2026-05-01", value: 0.9, count: 2 },
            { key: "2026-05-02", value: 0.85, count: 3 },
          ],
        }),
        { status: 200 },
      );
    });

    const rows = await fetchEvalAggregate(
      {
        workspace: "test-ws",
        groupBy: "time:day",
        metric: "avg_score",
        evalId: "acc",
        agentName: "chatbot",
        promptpackName: "v2",
        evalType: "llm_judge",
        from: new Date("2026-05-01T00:00:00Z"),
        to: new Date("2026-05-02T00:00:00Z"),
      },
      fakeFetch as unknown as typeof fetch,
    );

    expect(rows).toHaveLength(2);
    expect(rows[0]).toEqual({ key: "2026-05-01", value: 0.9, count: 2 });

    expect(calls).toHaveLength(1);
    const url = calls[0].url;
    expect(url.startsWith("/api/workspaces/test-ws/eval-results/aggregate?")).toBe(true);
    expect(url).toContain("groupBy=time%3Aday");
    expect(url).toContain("metric=avg_score");
    expect(url).toContain("evalId=acc");
    expect(url).toContain("agentName=chatbot");
    expect(url).toContain("promptpackName=v2");
    expect(url).toContain("evalType=llm_judge");
    expect(url).toContain("from=2026-05-01T00%3A00%3A00.000Z");
    expect(url).toContain("to=2026-05-02T00%3A00%3A00.000Z");
  });

  it("encodes workspace names with special characters in the path", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response(JSON.stringify({ rows: [] }), { status: 200 }),
    );

    await fetchEvalAggregate(
      { workspace: "ws/with-slash", groupBy: "eval_id", metric: "count" },
      fakeFetch as unknown as typeof fetch,
    );

    const url = String((fakeFetch.mock.calls as unknown as [string][])[0][0]);
    expect(url).toContain("/api/workspaces/ws%2Fwith-slash/");
  });

  it("returns [] when the body has no rows field", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response(JSON.stringify({}), { status: 200 }),
    );

    const rows = await fetchEvalAggregate(
      { workspace: "ws", groupBy: "eval_id", metric: "count" },
      fakeFetch as unknown as typeof fetch,
    );
    expect(rows).toEqual([]);
  });

  it("throws on non-2xx response", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response("nope", { status: 500, statusText: "Internal Server Error" }),
    );

    await expect(
      fetchEvalAggregate(
        { workspace: "ws", groupBy: "eval_id", metric: "count" },
        fakeFetch as unknown as typeof fetch,
      ),
    ).rejects.toThrow(/eval-aggregate: 500/);
  });

  it("omits optional params from the query when not provided", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response(JSON.stringify({ rows: [] }), { status: 200 }),
    );

    await fetchEvalAggregate(
      { workspace: "ws", groupBy: "eval_id", metric: "count" },
      fakeFetch as unknown as typeof fetch,
    );

    const url = String((fakeFetch.mock.calls as unknown as [string][])[0][0]);
    expect(url).not.toContain("agentName=");
    expect(url).not.toContain("evalId=");
    expect(url).not.toContain("from=");
    expect(url).not.toContain("to=");
  });
});

describe("classifyEvalType", () => {
  it("maps boolean-style eval handlers to 'boolean'", () => {
    expect(classifyEvalType("assertion")).toBe("boolean");
    expect(classifyEvalType("regex")).toBe("boolean");
    expect(classifyEvalType("json_path")).toBe("boolean");
  });

  it("maps llm-judge eval handlers to 'gauge'", () => {
    expect(classifyEvalType("llm_judge")).toBe("gauge");
    expect(classifyEvalType("llm_judge_session")).toBe("gauge");
  });

  it("defaults unknown eval types to 'gauge'", () => {
    expect(classifyEvalType("")).toBe("gauge");
    expect(classifyEvalType("future_handler")).toBe("gauge");
  });
});

describe("fetchEvalDiscovery", () => {
  it("hits the workspace-scoped /discover endpoint and returns full payload", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response(
        JSON.stringify({
          evals: [{ evalId: "acc", evalType: "llm_judge" }],
          agents: ["agent-a", "agent-b"],
          promptpacks: ["pack-v1"],
        }),
        { status: 200 },
      ),
    );

    const res = await fetchEvalDiscovery("test-ws", fakeFetch as unknown as typeof fetch);

    expect(res.evals).toHaveLength(1);
    expect(res.agents).toEqual(["agent-a", "agent-b"]);
    expect(res.promptpacks).toEqual(["pack-v1"]);

    const url = String((fakeFetch.mock.calls as unknown as [string][])[0][0]);
    expect(url).toBe("/api/workspaces/test-ws/eval-results/discover");
  });

  it("normalises missing slices to empty arrays", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response(JSON.stringify({}), { status: 200 }),
    );
    const res = await fetchEvalDiscovery("ws", fakeFetch as unknown as typeof fetch);
    expect(res).toEqual({ evals: [], agents: [], promptpacks: [] });
  });

  it("encodes workspace names with special characters in the path", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response(JSON.stringify({ evals: [], agents: [], promptpacks: [] }), {
        status: 200,
      }),
    );

    await fetchEvalDiscovery("ws/with-slash", fakeFetch as unknown as typeof fetch);

    const url = String((fakeFetch.mock.calls as unknown as [string][])[0][0]);
    expect(url).toContain("/api/workspaces/ws%2Fwith-slash/");
  });

  it("throws on non-2xx response", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response("nope", { status: 500, statusText: "Internal Server Error" }),
    );

    await expect(
      fetchEvalDiscovery("ws", fakeFetch as unknown as typeof fetch),
    ).rejects.toThrow(/eval-discover: 500/);
  });
});

describe("fetchEvalDescriptors", () => {
  it("requests the workspace-scoped discover endpoint and returns evals", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response(
        JSON.stringify({
          evals: [
            { evalId: "acc", evalType: "llm_judge" },
            { evalId: "lat", evalType: "assertion" },
          ],
        }),
        { status: 200 },
      ),
    );

    const evals = await fetchEvalDescriptors("test-ws", fakeFetch as unknown as typeof fetch);

    expect(evals).toEqual([
      { evalId: "acc", evalType: "llm_judge" },
      { evalId: "lat", evalType: "assertion" },
    ]);
    const url = String((fakeFetch.mock.calls as unknown as [string][])[0][0]);
    expect(url).toBe("/api/workspaces/test-ws/eval-results/discover");
  });

  it("returns [] when the body has no evals field", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response(JSON.stringify({}), { status: 200 }),
    );
    const evals = await fetchEvalDescriptors("ws", fakeFetch as unknown as typeof fetch);
    expect(evals).toEqual([]);
  });

  it("throws on non-2xx response", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response("denied", { status: 403, statusText: "Forbidden" }),
    );

    await expect(
      fetchEvalDescriptors("ws", fakeFetch as unknown as typeof fetch),
    ).rejects.toThrow(/eval-discover: 403/);
  });
});

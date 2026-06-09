/**
 * Tests for the server-side workspace cost fetch.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi } from "vitest";
import { fetchWorkspaceCostData } from "./cost-from-session-api";

function jsonResponse(rows: Array<{ key: string; value: number; count: number }>) {
  return new Response(JSON.stringify({ rows }), { status: 200 });
}

describe("fetchWorkspaceCostData", () => {
  it("issues one aggregate call per metric + the time series and assembles CostData", async () => {
    const calls: string[] = [];
    const fetchImpl = vi.fn(async (url: string) => {
      calls.push(url);
      if (url.includes("metric=sum_cost_usd") && url.includes("time%3Ahour")) {
        return jsonResponse([{ key: "2026-06-09T13:00:00Z|openai", value: 0.03, count: 2 }]);
      }
      if (url.includes("metric=sum_cost_usd")) {
        return jsonResponse([{ key: "openai|gpt-4|chatbot", value: 0.03, count: 2 }]);
      }
      if (url.includes("metric=sum_input_tokens")) {
        return jsonResponse([{ key: "openai|gpt-4|chatbot", value: 150, count: 2 }]);
      }
      if (url.includes("metric=sum_output_tokens")) {
        return jsonResponse([{ key: "openai|gpt-4|chatbot", value: 15, count: 2 }]);
      }
      if (url.includes("metric=sum_cached_tokens")) {
        return jsonResponse([{ key: "openai|gpt-4|chatbot", value: 0, count: 2 }]);
      }
      return jsonResponse([{ key: "openai|gpt-4|chatbot", value: 2, count: 2 }]); // count
    }) as unknown as typeof fetch;

    const data = await fetchWorkspaceCostData(
      [{ sessionURL: "https://session-default:8080", namespace: "omnia-default" }],
      new Date("2026-06-08T13:00:00Z"),
      new Date("2026-06-09T13:00:00Z"),
      fetchImpl,
    );

    expect(data.available).toBe(true);
    expect(data.summary.totalCost).toBeCloseTo(0.03, 9);
    expect(data.summary.totalTokens).toBe(165);
    expect(data.byAgent).toHaveLength(1);
    expect(data.timeSeries[0].byProvider.openai).toBeCloseTo(0.03, 9);
    // every call pins the resolved namespace, not the workspace name.
    expect(calls.every((u) => u.includes("namespace=omnia-default"))).toBe(true);
    // 6 calls: 5 matrix metrics + 1 series.
    expect(calls).toHaveLength(6);
  });

  it("strips a trailing slash from the session URL", async () => {
    const calls: string[] = [];
    const fetchImpl = vi.fn(async (url: string) => {
      calls.push(url);
      return jsonResponse([]);
    }) as unknown as typeof fetch;

    await fetchWorkspaceCostData(
      [{ sessionURL: "https://session-default:8080/", namespace: "omnia-default" }],
      new Date("2026-06-08T13:00:00Z"),
      new Date("2026-06-09T13:00:00Z"),
      fetchImpl,
    );
    expect(calls[0].startsWith("https://session-default:8080/api/v1/provider-calls/aggregate?")).toBe(true);
  });

  it("returns available:false when every source fails", async () => {
    const fetchImpl = vi.fn(async () => {
      throw new Error("connection refused");
    }) as unknown as typeof fetch;

    const data = await fetchWorkspaceCostData(
      [{ sessionURL: "https://session-default:8080", namespace: "omnia-default" }],
      new Date("2026-06-08T13:00:00Z"),
      new Date("2026-06-09T13:00:00Z"),
      fetchImpl,
    );
    expect(data.available).toBe(false);
    expect(data.reason).toBeTruthy();
    expect(data.byAgent).toEqual([]);
  });

  it("treats a missing rows field as empty (available, zero totals)", async () => {
    const fetchImpl = vi.fn(async () =>
      new Response(JSON.stringify({}), { status: 200 }),
    ) as unknown as typeof fetch;

    const data = await fetchWorkspaceCostData(
      [{ sessionURL: "https://session-default:8080", namespace: "omnia-default" }],
      new Date("2026-06-08T13:00:00Z"),
      new Date("2026-06-09T13:00:00Z"),
      fetchImpl,
    );
    expect(data.available).toBe(true);
    expect(data.summary.totalCost).toBe(0);
    expect(data.byAgent).toEqual([]);
  });

  it("returns available:false for an empty source list", async () => {
    const fetchImpl = vi.fn() as unknown as typeof fetch;
    const data = await fetchWorkspaceCostData(
      [],
      new Date("2026-06-08T13:00:00Z"),
      new Date("2026-06-09T13:00:00Z"),
      fetchImpl,
    );
    expect(data.available).toBe(false);
    expect(data.reason).toBe("Session API unavailable");
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it("returns available:false when a source responds non-2xx", async () => {
    const fetchImpl = vi.fn(async () =>
      new Response("err", { status: 500, statusText: "Internal Server Error" }),
    ) as unknown as typeof fetch;

    const data = await fetchWorkspaceCostData(
      [{ sessionURL: "https://session-default:8080", namespace: "omnia-default" }],
      new Date("2026-06-08T13:00:00Z"),
      new Date("2026-06-09T13:00:00Z"),
      fetchImpl,
    );
    expect(data.available).toBe(false);
  });
});

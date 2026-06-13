/**
 * Tests for the server-side workspace cost fetch.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock the SA header builder so we can assert the outbound Authorization header
// without depending on a real projected token file.
vi.mock("@/lib/auth/session-api-token", () => ({
  serviceApiHeaders: vi.fn(
    (extra?: Record<string, string>): Record<string, string> => ({ ...extra }),
  ),
}));

import { fetchWorkspaceCostData } from "./cost-from-session-api";
import { serviceApiHeaders } from "@/lib/auth/session-api-token";

/** Make serviceApiHeaders behave as if a token (or none) is present. */
function stubToken(token: string): void {
  vi.mocked(serviceApiHeaders).mockImplementation(
    (extra?: Record<string, string>): Record<string, string> => {
      const headers: Record<string, string> = { ...extra };
      if (token) headers.Authorization = `Bearer ${token}`;
      return headers;
    },
  );
}

function jsonResponse(rows: Array<{ key: string; value: number; count: number }>) {
  return new Response(JSON.stringify({ rows }), { status: 200 });
}

describe("fetchWorkspaceCostData", () => {
  beforeEach(() => {
    stubToken("");
  });

  afterEach(() => {
    stubToken("");
  });

  it("attaches the SA bearer token to outbound requests when one is present", async () => {
    stubToken("sa-jwt");
    const inits: Array<RequestInit | undefined> = [];
    const fetchImpl = vi.fn(async (_url: string, init?: RequestInit) => {
      inits.push(init);
      return jsonResponse([]);
    }) as unknown as typeof fetch;

    await fetchWorkspaceCostData(
      [{ sessionURL: "https://session-default:8080", namespace: "omnia-default" }],
      new Date("2026-06-08T13:00:00Z"),
      new Date("2026-06-09T13:00:00Z"),
      fetchImpl,
    );

    expect(inits.length).toBeGreaterThan(0);
    for (const init of inits) {
      const headers = init?.headers as Record<string, string>;
      expect(headers.Authorization).toBe("Bearer sa-jwt");
      expect(headers.Accept).toBe("application/json");
    }
  });

  it("omits the Authorization header when no SA token is available (local dev)", async () => {
    stubToken("");
    let capturedInit: RequestInit | undefined;
    const fetchImpl = vi.fn(async (_url: string, init?: RequestInit) => {
      capturedInit = init;
      return jsonResponse([]);
    }) as unknown as typeof fetch;

    await fetchWorkspaceCostData(
      [{ sessionURL: "https://session-default:8080", namespace: "omnia-default" }],
      new Date("2026-06-08T13:00:00Z"),
      new Date("2026-06-09T13:00:00Z"),
      fetchImpl,
    );

    const headers = capturedInit?.headers as Record<string, string>;
    expect(headers.Authorization).toBeUndefined();
    expect(headers.Accept).toBe("application/json");
  });

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

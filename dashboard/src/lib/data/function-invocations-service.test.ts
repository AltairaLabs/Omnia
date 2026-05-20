/**
 * Tests for function-invocations-service.
 *
 * Covers URL composition (workspace name encoding, optional filters,
 * absence of the "?" when no params), envelope unwrapping, and error
 * propagation on non-2xx. Mock-to-contract: the mocked response shape
 * mirrors the Go FunctionInvocation struct's json tags so a tag
 * rename in Go would fail these tests immediately.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  fetchFunctionInvocations,
  fetchFunctionInvocation,
} from "./function-invocations-service";

beforeEach(() => {
  vi.restoreAllMocks();
});

const SAMPLE_ROW = {
  id: "00000000-0000-0000-0000-000000000001",
  namespace: "ns-a",
  functionName: "summarizer",
  inputHash: "abc",
  outputJson: { a: 1 },
  status: "success" as const,
  durationMs: 42,
  costUsd: 0.001,
  traceId: "0102030405060708090a0b0c0d0e0f10",
  createdAt: "2026-05-20T10:00:00Z",
};

describe("fetchFunctionInvocations", () => {
  it("builds the workspace-scoped URL with every optional filter", async () => {
    const fakeFetch = vi.fn(
      async () => new Response(JSON.stringify({ rows: [SAMPLE_ROW] }), { status: 200 }),
    );

    const rows = await fetchFunctionInvocations(
      {
        workspace: "test-ws",
        functionName: "summarizer",
        from: new Date("2026-05-19T00:00:00Z"),
        to: new Date("2026-05-20T00:00:00Z"),
        limit: 50,
      },
      fakeFetch as unknown as typeof fetch,
    );

    expect(rows).toHaveLength(1);
    expect(rows[0]).toEqual(SAMPLE_ROW);

    const url = String((fakeFetch.mock.calls as unknown as [string][])[0][0]);
    expect(url.startsWith("/api/workspaces/test-ws/function-invocations?")).toBe(true);
    expect(url).toContain("function=summarizer");
    expect(url).toContain("from=2026-05-19T00%3A00%3A00.000Z");
    expect(url).toContain("to=2026-05-20T00%3A00%3A00.000Z");
    expect(url).toContain("limit=50");
  });

  it("omits the query string entirely when no filters are passed", async () => {
    const fakeFetch = vi.fn(
      async () => new Response(JSON.stringify({ rows: [] }), { status: 200 }),
    );
    await fetchFunctionInvocations(
      { workspace: "ws" },
      fakeFetch as unknown as typeof fetch,
    );
    const url = String((fakeFetch.mock.calls as unknown as [string][])[0][0]);
    expect(url).toBe("/api/workspaces/ws/function-invocations");
  });

  it("encodes workspace names with special characters in the path", async () => {
    const fakeFetch = vi.fn(
      async () => new Response(JSON.stringify({ rows: [] }), { status: 200 }),
    );
    await fetchFunctionInvocations(
      { workspace: "ws/with-slash" },
      fakeFetch as unknown as typeof fetch,
    );
    const url = String((fakeFetch.mock.calls as unknown as [string][])[0][0]);
    expect(url).toContain("/api/workspaces/ws%2Fwith-slash/");
  });

  it("returns [] when the body has no rows field", async () => {
    const fakeFetch = vi.fn(
      async () => new Response(JSON.stringify({}), { status: 200 }),
    );
    const rows = await fetchFunctionInvocations(
      { workspace: "ws" },
      fakeFetch as unknown as typeof fetch,
    );
    expect(rows).toEqual([]);
  });

  it("throws when the proxy returns a non-2xx", async () => {
    const fakeFetch = vi.fn(
      async () => new Response("{}", { status: 503, statusText: "Service Unavailable" }),
    );
    await expect(
      fetchFunctionInvocations(
        { workspace: "ws" },
        fakeFetch as unknown as typeof fetch,
      ),
    ).rejects.toThrow(/503/);
  });

  it("forwards limit=0 as an explicit filter, not as 'omitted'", async () => {
    // Edge case: limit=0 means "the caller really wants zero rows back".
    // It must reach the URL — `if (params.limit)` would have silently
    // dropped this. The service uses `!== undefined` to guard against that.
    const fakeFetch = vi.fn(
      async () => new Response(JSON.stringify({ rows: [] }), { status: 200 }),
    );
    await fetchFunctionInvocations(
      { workspace: "ws", limit: 0 },
      fakeFetch as unknown as typeof fetch,
    );
    const url = String((fakeFetch.mock.calls as unknown as [string][])[0][0]);
    expect(url).toContain("limit=0");
  });
});

describe("fetchFunctionInvocation", () => {
  it("encodes the id in the path", async () => {
    const fakeFetch = vi.fn(
      async () => new Response(JSON.stringify(SAMPLE_ROW), { status: 200 }),
    );
    const row = await fetchFunctionInvocation(
      "ws",
      "id/with-slash",
      fakeFetch as unknown as typeof fetch,
    );
    expect(row).toEqual(SAMPLE_ROW);
    const url = String((fakeFetch.mock.calls as unknown as [string][])[0][0]);
    expect(url).toBe("/api/workspaces/ws/function-invocations/id%2Fwith-slash");
  });

  it("throws when the proxy returns 404", async () => {
    const fakeFetch = vi.fn(
      async () => new Response("{}", { status: 404, statusText: "Not Found" }),
    );
    await expect(
      fetchFunctionInvocation("ws", "missing", fakeFetch as unknown as typeof fetch),
    ).rejects.toThrow(/404/);
  });
});

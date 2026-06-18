/**
 * Tests for provider-calls-service.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  fetchProviderCallsAggregate,
  fetchProviderCallsDiscovery,
} from "./provider-calls-service";

beforeEach(() => {
  vi.restoreAllMocks();
});

describe("fetchProviderCallsAggregate", () => {
  it("builds the workspace-scoped URL with required + optional params", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response(
        JSON.stringify({
          rows: [
            { key: "openai", value: 0.031, count: 3 },
            { key: "anthropic", value: 0.05, count: 1 },
          ],
        }),
        { status: 200 },
      ),
    );

    const rows = await fetchProviderCallsAggregate(
      {
        workspace: "test-ws",
        groupBy: "provider",
        metric: "sum_cost_usd",
        agentName: "chatbot",
        provider: "openai",
        model: "gpt-4",
        from: new Date("2026-05-01T00:00:00Z"),
        to: new Date("2026-05-02T00:00:00Z"),
      },
      fakeFetch as unknown as typeof fetch,
    );

    expect(rows).toHaveLength(2);
    expect(rows[0]).toEqual({ key: "openai", value: 0.031, count: 3 });

    const url = String((fakeFetch.mock.calls as unknown as [string][])[0][0]);
    expect(url.startsWith("/api/workspaces/test-ws/provider-calls/aggregate?")).toBe(true);
    expect(url).toContain("groupBy=provider");
    expect(url).toContain("metric=sum_cost_usd");
    expect(url).toContain("agentName=chatbot");
    expect(url).toContain("provider=openai");
    expect(url).toContain("model=gpt-4");
    expect(url).toContain("from=2026-05-01T00%3A00%3A00.000Z");
    expect(url).toContain("to=2026-05-02T00%3A00%3A00.000Z");
  });

  it("emits the providerName filter when set", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response(JSON.stringify({ rows: [] }), { status: 200 }),
    );
    await fetchProviderCallsAggregate(
      { workspace: "ws", groupBy: "provider_name", metric: "count", providerName: "openai-primary" },
      fakeFetch as unknown as typeof fetch,
    );
    const url = String((fakeFetch.mock.calls as unknown as [string][])[0][0]);
    expect(url).toContain("groupBy=provider_name");
    expect(url).toContain("providerName=openai-primary");
  });

  it("encodes workspace names with special characters in the path", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response(JSON.stringify({ rows: [] }), { status: 200 }),
    );
    await fetchProviderCallsAggregate(
      { workspace: "ws/with-slash", groupBy: "provider", metric: "count" },
      fakeFetch as unknown as typeof fetch,
    );
    const url = String((fakeFetch.mock.calls as unknown as [string][])[0][0]);
    expect(url).toContain("/api/workspaces/ws%2Fwith-slash/");
  });

  it("returns [] when the body has no rows field", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response(JSON.stringify({}), { status: 200 }),
    );
    const rows = await fetchProviderCallsAggregate(
      { workspace: "ws", groupBy: "provider", metric: "count" },
      fakeFetch as unknown as typeof fetch,
    );
    expect(rows).toEqual([]);
  });

  it("returns [] on a non-2xx response (cost data is non-critical)", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response("nope", { status: 500, statusText: "Internal Server Error" }),
    );
    const rows = await fetchProviderCallsAggregate(
      { workspace: "ws", groupBy: "provider", metric: "count" },
      fakeFetch as unknown as typeof fetch,
    );
    expect(rows).toEqual([]);
  });

  it("degrades to the proxy's empty rows on an unavailable (non-2xx) response", async () => {
    // The proxy returns { rows: [] } with a 503/502 when session-api is not
    // configured/unreachable; the client should honour it rather than throw.
    const fakeFetch = vi.fn(async () =>
      new Response(JSON.stringify({ error: "Session API not configured", rows: [] }), { status: 503 }),
    );
    const rows = await fetchProviderCallsAggregate(
      { workspace: "ws", groupBy: "provider", metric: "count" },
      fakeFetch as unknown as typeof fetch,
    );
    expect(rows).toEqual([]);
  });

  it("joins an array groupBy with commas", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response(JSON.stringify({ rows: [] }), { status: 200 }),
    );
    await fetchProviderCallsAggregate(
      { workspace: "ws", groupBy: ["time:hour", "provider"], metric: "sum_cost_usd" },
      fakeFetch as unknown as typeof fetch,
    );
    const url = String((fakeFetch.mock.calls as unknown as [string][])[0][0]);
    expect(url).toContain("groupBy=time%3Ahour%2Cprovider");
  });

  it("omits optional params from the query when not provided", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response(JSON.stringify({ rows: [] }), { status: 200 }),
    );
    await fetchProviderCallsAggregate(
      { workspace: "ws", groupBy: "provider", metric: "count" },
      fakeFetch as unknown as typeof fetch,
    );
    const url = String((fakeFetch.mock.calls as unknown as [string][])[0][0]);
    expect(url).not.toContain("agentName=");
    expect(url).not.toContain("provider=");
    expect(url).not.toContain("model=");
    expect(url).not.toContain("from=");
    expect(url).not.toContain("to=");
  });
});

describe("fetchProviderCallsDiscovery", () => {
  it("returns providers + provider names + models", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response(
        JSON.stringify({
          providers: ["anthropic", "openai"],
          providerNames: ["openai-cheap", "openai-primary"],
          models: ["claude-3-5-sonnet", "gpt-4"],
        }),
        { status: 200 },
      ),
    );
    const res = await fetchProviderCallsDiscovery("test-ws", fakeFetch as unknown as typeof fetch);
    expect(res.providers).toEqual(["anthropic", "openai"]);
    expect(res.providerNames).toEqual(["openai-cheap", "openai-primary"]);
    expect(res.models).toEqual(["claude-3-5-sonnet", "gpt-4"]);

    const url = String((fakeFetch.mock.calls as unknown as [string][])[0][0]);
    expect(url).toBe("/api/workspaces/test-ws/provider-calls/discover");
  });

  it("normalises missing slices to empty arrays", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response(JSON.stringify({}), { status: 200 }),
    );
    const res = await fetchProviderCallsDiscovery("ws", fakeFetch as unknown as typeof fetch);
    expect(res).toEqual({ providers: [], providerNames: [], models: [] });
  });

  it("returns empty slices on a non-2xx response", async () => {
    const fakeFetch = vi.fn(async () =>
      new Response("denied", { status: 403, statusText: "Forbidden" }),
    );
    const res = await fetchProviderCallsDiscovery("ws", fakeFetch as unknown as typeof fetch);
    expect(res).toEqual({ providers: [], providerNames: [], models: [] });
  });
});

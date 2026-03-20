/**
 * Tests for session namespace guard.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("@/lib/k8s/workspace-route-helpers", () => ({
  getWorkspace: vi.fn(),
}));

const mockFetch = vi.fn();
global.fetch = mockFetch;

describe("verifySessionNamespace", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("SESSION_API_URL", "https://session-api:8080");
  });

  afterEach(() => {
    vi.resetAllMocks();
    vi.unstubAllEnvs();
  });

  it("returns ok when session namespace matches workspace namespace", async () => {
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue({
      metadata: { name: "test-ws" },
      spec: { namespace: { name: "team-a-ns" } },
    } as ReturnType<typeof getWorkspace> extends Promise<infer T> ? NonNullable<T> : never);

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ session: { id: "sess-1", namespace: "team-a-ns" } }),
    });

    const { verifySessionNamespace } = await import("./session-namespace-guard");
    const result = await verifySessionNamespace("test-ws", "sess-1");

    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.namespace).toBe("team-a-ns");
      expect(result.baseUrl).toBe("https://session-api:8080");
    }
  });

  it("returns 404 when session namespace does not match workspace (IDOR)", async () => {
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue({
      metadata: { name: "workspace-a" },
      spec: { namespace: { name: "namespace-a" } },
    } as ReturnType<typeof getWorkspace> extends Promise<infer T> ? NonNullable<T> : never);

    // Session belongs to namespace-b, not namespace-a
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ session: { id: "sess-from-b", namespace: "namespace-b" } }),
    });

    const { verifySessionNamespace } = await import("./session-namespace-guard");
    const result = await verifySessionNamespace("workspace-a", "sess-from-b");

    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.response.status).toBe(404);
      const body = await result.response.json();
      expect(body.error).toBe("Session not found");
    }
  });

  it("returns 503 when SESSION_API_URL is not set", async () => {
    vi.stubEnv("SESSION_API_URL", "");

    const { verifySessionNamespace } = await import("./session-namespace-guard");
    const result = await verifySessionNamespace("test-ws", "sess-1");

    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.response.status).toBe(503);
    }
  });

  it("returns 404 when workspace does not exist", async () => {
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(null);

    const { verifySessionNamespace } = await import("./session-namespace-guard");
    const result = await verifySessionNamespace("no-such-ws", "sess-1");

    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.response.status).toBe(404);
      const body = await result.response.json();
      expect(body.error).toBe("Workspace not found");
    }
  });

  it("forwards backend 404 when session does not exist", async () => {
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue({
      metadata: { name: "test-ws" },
      spec: { namespace: { name: "test-ns" } },
    } as ReturnType<typeof getWorkspace> extends Promise<infer T> ? NonNullable<T> : never);

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
      json: () => Promise.resolve({ error: "session not found" }),
    });

    const { verifySessionNamespace } = await import("./session-namespace-guard");
    const result = await verifySessionNamespace("test-ws", "no-such-session");

    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.response.status).toBe(404);
    }
  });

  it("returns 502 when session-api fetch fails", async () => {
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue({
      metadata: { name: "test-ws" },
      spec: { namespace: { name: "test-ns" } },
    } as ReturnType<typeof getWorkspace> extends Promise<infer T> ? NonNullable<T> : never);

    mockFetch.mockRejectedValueOnce(new Error("Connection refused"));

    const { verifySessionNamespace } = await import("./session-namespace-guard");
    const result = await verifySessionNamespace("test-ws", "sess-1");

    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.response.status).toBe(502);
    }
  });

  it("strips trailing slash from SESSION_API_URL", async () => {
    vi.stubEnv("SESSION_API_URL", "https://session-api:8080/");

    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue({
      metadata: { name: "test-ws" },
      spec: { namespace: { name: "test-ns" } },
    } as ReturnType<typeof getWorkspace> extends Promise<infer T> ? NonNullable<T> : never);

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ session: { id: "sess-1", namespace: "test-ns" } }),
    });

    const { verifySessionNamespace } = await import("./session-namespace-guard");
    const result = await verifySessionNamespace("test-ws", "sess-1");

    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.baseUrl).toBe("https://session-api:8080");
    }

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).not.toContain("//api");
  });
});

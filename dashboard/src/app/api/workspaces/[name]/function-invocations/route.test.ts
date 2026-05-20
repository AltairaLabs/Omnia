/**
 * Tests for the workspace function-invocations list proxy route.
 *
 * Pins: workspace name pinned to session-api's namespace filter,
 * forwarded params, 503 on URL-resolve miss, 502 on session-api
 * connection failure, and the security invariant that a caller
 * cannot override the namespace via the query string.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({ getUser: vi.fn() }));
vi.mock("@/lib/auth/workspace-authz", () => ({ checkWorkspaceAccess: vi.fn() }));
vi.mock("@/lib/k8s/service-url-resolver", () => ({ resolveServiceURLs: vi.fn() }));

const mockUser = {
  id: "u1",
  provider: "oauth" as const,
  username: "u",
  email: "u@example.com",
  groups: ["users"],
  role: "viewer" as const,
};
const viewerPermissions = { read: true, write: false, delete: false, manageMembers: false };

const mockFetch = vi.fn();
global.fetch = mockFetch;

function makeRequest(qs = ""): NextRequest {
  const sep = qs ? "?" : "";
  return new NextRequest(
    `http://localhost:3000/api/workspaces/test-ws/function-invocations${sep}${qs}`,
    { method: "GET" },
  );
}

function makeContext() {
  return { params: Promise.resolve({ name: "test-ws" }) };
}

async function setupAuth() {
  const { getUser } = await import("@/lib/auth");
  const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
  const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");
  vi.mocked(resolveServiceURLs).mockResolvedValue({
    sessionURL: "https://session-api:8080",
    memoryURL: "https://memory-api:8080",
  });
  vi.mocked(getUser).mockResolvedValue(mockUser);
  vi.mocked(checkWorkspaceAccess).mockResolvedValue({
    granted: true,
    role: "viewer",
    permissions: viewerPermissions,
  });
}

describe("GET /api/workspaces/[name]/function-invocations", () => {
  beforeEach(() => vi.resetModules());
  afterEach(() => vi.resetAllMocks());

  it("pins namespace to the workspace name and forwards optional filters", async () => {
    await setupAuth();
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ rows: [] }),
    });

    const { GET } = await import("./route");
    const response = await GET(
      makeRequest("function=summarizer&from=2026-05-01T00:00:00Z&limit=50"),
      makeContext(),
    );
    expect(response.status).toBe(200);

    const url = mockFetch.mock.calls[0][0] as string;
    expect(url).toContain("https://session-api:8080/api/v1/function-invocations?");
    expect(url).toContain("namespace=test-ws");
    expect(url).toContain("function=summarizer");
    expect(url).toContain("from=2026-05-01T00%3A00%3A00Z");
    expect(url).toContain("limit=50");
  });

  it("refuses to forward a caller-supplied namespace override", async () => {
    await setupAuth();
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ rows: [] }),
    });

    const { GET } = await import("./route");
    // Malicious caller passes ?namespace=other-tenant — the proxy must
    // ignore it and keep namespace pinned to the workspace name.
    await GET(makeRequest("namespace=other-tenant"), makeContext());

    const url = mockFetch.mock.calls[0][0] as string;
    expect(url).toContain("namespace=test-ws");
    expect(url).not.toContain("namespace=other-tenant");
  });

  it("returns 503 with empty rows when session-api URL is unresolved", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");
    vi.mocked(resolveServiceURLs).mockResolvedValue(null);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const { GET } = await import("./route");
    const response = await GET(makeRequest(), makeContext());
    expect(response.status).toBe(503);
    const body = await response.json();
    expect(body.rows).toEqual([]);
  });

  it("returns 502 on session-api connection failure", async () => {
    await setupAuth();
    mockFetch.mockRejectedValueOnce(new Error("ECONNREFUSED"));

    const { GET } = await import("./route");
    const response = await GET(makeRequest(), makeContext());
    expect(response.status).toBe(502);
    const body = await response.json();
    expect(body.rows).toEqual([]);
    expect(body.details).toContain("ECONNREFUSED");
  });

  it("strips trailing slash on the session-api base URL", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");
    vi.mocked(resolveServiceURLs).mockResolvedValue({
      sessionURL: "https://session-api:8080/",
      memoryURL: "https://memory-api:8080",
    });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ rows: [] }),
    });

    const { GET } = await import("./route");
    await GET(makeRequest(), makeContext());
    const url = mockFetch.mock.calls[0][0] as string;
    expect(url.startsWith("https://session-api:8080/api/v1/")).toBe(true);
  });
});

/**
 * Tests for workspace provider-calls aggregate proxy route.
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

function makeRequest(query: string): NextRequest {
  return new NextRequest(
    `http://localhost:3000/api/workspaces/test-ws/provider-calls/aggregate?${query}`,
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
    memoryURL: "https://memory-api:8080", namespace: "omnia-test", privacyURL: ""
  });
  vi.mocked(getUser).mockResolvedValue(mockUser);
  vi.mocked(checkWorkspaceAccess).mockResolvedValue({
    granted: true,
    role: "viewer",
    permissions: viewerPermissions,
  });
}

describe("GET /api/workspaces/[name]/provider-calls/aggregate", () => {
  beforeEach(() => vi.resetModules());
  afterEach(() => vi.resetAllMocks());

  it("forwards query and pins namespace", async () => {
    await setupAuth();
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ rows: [{ key: "openai", value: 0.031, count: 3 }] }),
    });

    const { GET } = await import("./route");
    const response = await GET(
      makeRequest("groupBy=provider&metric=sum_cost_usd"),
      makeContext(),
    );

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.rows).toHaveLength(1);

    const url = mockFetch.mock.calls[0][0] as string;
    expect(url).toContain("session-api:8080/api/v1/provider-calls/aggregate?");
    expect(url).toContain("namespace=omnia-test");
    expect(url).toContain("groupBy=provider");
    expect(url).toContain("metric=sum_cost_usd");
  });

  it("overrides caller-supplied namespace with the resolved workspace namespace", async () => {
    await setupAuth();
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ rows: [] }),
    });

    const { GET } = await import("./route");
    await GET(
      makeRequest("namespace=other-ws&groupBy=provider&metric=count"),
      makeContext(),
    );
    const url = mockFetch.mock.calls[0][0] as string;
    expect(url).toContain("namespace=omnia-test");
    expect(url).not.toContain("namespace=other-ws");
    // #1257: must use the resolved namespace, never the workspace NAME.
    expect(url).not.toContain("namespace=test-ws");
  });

  it("returns 503 when session-api URL cannot be resolved", async () => {
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
    const response = await GET(makeRequest("groupBy=provider&metric=count"), makeContext());
    expect(response.status).toBe(503);
    const body = await response.json();
    expect(body.rows).toEqual([]);
  });

  it("returns 502 when fetch throws", async () => {
    await setupAuth();
    mockFetch.mockRejectedValueOnce(new Error("ECONNREFUSED"));
    const { GET } = await import("./route");
    const response = await GET(makeRequest("groupBy=provider&metric=count"), makeContext());
    expect(response.status).toBe(502);
    const body = await response.json();
    expect(body.rows).toEqual([]);
  });
});

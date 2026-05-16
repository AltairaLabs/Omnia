/**
 * Tests for workspace eval-results aggregate proxy route.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/k8s/service-url-resolver", () => ({
  resolveServiceURLs: vi.fn(),
}));

const mockUser = {
  id: "testuser-id",
  provider: "oauth" as const,
  username: "testuser",
  email: "test@example.com",
  groups: ["users"],
  role: "viewer" as const,
};

const viewerPermissions = { read: true, write: false, delete: false, manageMembers: false };

const mockFetch = vi.fn();
global.fetch = mockFetch;

function makeRequest(query: string): NextRequest {
  return new NextRequest(
    `http://localhost:3000/api/workspaces/test-ws/eval-results/aggregate?${query}`,
    { method: "GET" }
  );
}

function makeContext() {
  return { params: Promise.resolve({ name: "test-ws" }) };
}

describe("GET /api/workspaces/[name]/eval-results/aggregate", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

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

  it("proxies aggregate request and injects namespace from workspace name", async () => {
    await setupAuth();

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () =>
        Promise.resolve({
          rows: [
            { key: "2026-05-01", value: 0.85, count: 2 },
            { key: "2026-05-02", value: 0.8, count: 2 },
          ],
        }),
    });

    const { GET } = await import("./route");
    const response = await GET(
      makeRequest("groupBy=time:day&metric=avg_score&evalId=acc"),
      makeContext()
    );

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.rows).toHaveLength(2);
    expect(body.rows[0].key).toBe("2026-05-01");

    // Verify the proxy forwarded query params AND pinned namespace=test-ws.
    expect(mockFetch).toHaveBeenCalledOnce();
    const calledURL = mockFetch.mock.calls[0][0] as string;
    expect(calledURL).toContain("session-api:8080/api/v1/eval-results/aggregate?");
    expect(calledURL).toContain("namespace=test-ws");
    expect(calledURL).toContain("groupBy=time%3Aday");
    expect(calledURL).toContain("metric=avg_score");
    expect(calledURL).toContain("evalId=acc");
  });

  it("overrides caller-supplied namespace with the workspace name", async () => {
    await setupAuth();

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ rows: [] }),
    });

    const { GET } = await import("./route");
    await GET(
      // Caller tries to read another workspace's data — should be ignored.
      makeRequest("namespace=other-ws&groupBy=eval_id&metric=count"),
      makeContext()
    );

    const calledURL = mockFetch.mock.calls[0][0] as string;
    expect(calledURL).toContain("namespace=test-ws");
    expect(calledURL).not.toContain("namespace=other-ws");
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
    const response = await GET(
      makeRequest("groupBy=time:day&metric=count"),
      makeContext()
    );

    expect(response.status).toBe(503);
    const body = await response.json();
    expect(body.rows).toEqual([]);
  });

  it("returns 502 when session-api fetch throws", async () => {
    await setupAuth();
    mockFetch.mockRejectedValueOnce(new Error("ECONNREFUSED"));

    const { GET } = await import("./route");
    const response = await GET(
      makeRequest("groupBy=time:day&metric=count"),
      makeContext()
    );

    expect(response.status).toBe(502);
    const body = await response.json();
    expect(body.error).toContain("Session API");
    expect(body.rows).toEqual([]);
  });
});

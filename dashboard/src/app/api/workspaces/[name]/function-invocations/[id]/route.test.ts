/**
 * Tests for the single function-invocation proxy route.
 *
 * Pins: workspace name pinned to session-api's namespace filter,
 * URL encoding for the id segment, 503 / 502 fallbacks.
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

function makeRequest(id = "inv-1"): NextRequest {
  return new NextRequest(
    `http://localhost:3000/api/workspaces/test-ws/function-invocations/${encodeURIComponent(id)}`,
    { method: "GET" },
  );
}

function makeContext(id = "inv-1") {
  return { params: Promise.resolve({ name: "test-ws", id }) };
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

describe("GET /api/workspaces/[name]/function-invocations/[id]", () => {
  beforeEach(() => vi.resetModules());
  afterEach(() => vi.resetAllMocks());

  it("forwards (namespace=workspace, id) to session-api", async () => {
    await setupAuth();
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () =>
        Promise.resolve({
          id: "inv-1",
          namespace: "test-ws",
          functionName: "summarizer",
          status: "success",
        }),
    });

    const { GET } = await import("./route");
    const response = await GET(makeRequest(), makeContext());
    expect(response.status).toBe(200);

    const url = mockFetch.mock.calls[0][0] as string;
    expect(url).toBe(
      "https://session-api:8080/api/v1/function-invocations/inv-1?namespace=test-ws",
    );
  });

  it("encodes ids with slashes / special chars in the path", async () => {
    await setupAuth();
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({}),
    });

    const { GET } = await import("./route");
    await GET(makeRequest("id/with-slash"), makeContext("id/with-slash"));

    const url = mockFetch.mock.calls[0][0] as string;
    expect(url).toContain("/function-invocations/id%2Fwith-slash");
  });

  it("returns 503 when session-api URL is unresolved", async () => {
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
  });

  it("returns 502 on session-api connection failure", async () => {
    await setupAuth();
    mockFetch.mockRejectedValueOnce(new Error("ECONNREFUSED"));

    const { GET } = await import("./route");
    const response = await GET(makeRequest(), makeContext());
    expect(response.status).toBe(502);
    const body = await response.json();
    expect(body.details).toContain("ECONNREFUSED");
  });

  it("propagates the upstream status (e.g. 404 cross-tenant)", async () => {
    await setupAuth();
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
      json: () => Promise.resolve({ error: "function invocation not found" }),
    });

    const { GET } = await import("./route");
    const response = await GET(makeRequest(), makeContext());
    expect(response.status).toBe(404);
  });
});

/**
 * Tests for eval results summary proxy route.
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

const mockUser = {
  id: "testuser-id",
  provider: "oauth" as const,
  username: "testuser",
  email: "test@example.com",
  groups: ["users"],
  role: "viewer" as const,
};

const viewerPermissions = { read: true, write: false, delete: false, manageMembers: false };
const noPermissions = { read: false, write: false, delete: false, manageMembers: false };

const mockFetch = vi.fn();
global.fetch = mockFetch;

function createMockRequest(query = ""): NextRequest {
  const url = `http://localhost:3000/api/workspaces/test-ws/eval-results/summary${query}`;
  return new NextRequest(url, { method: "GET" });
}

function createMockContext() {
  return { params: Promise.resolve({ name: "test-ws" }) };
}

describe("GET /api/workspaces/[name]/eval-results/summary", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("SESSION_API_URL", "https://session-api:8080");
  });

  afterEach(() => {
    vi.resetAllMocks();
    vi.unstubAllEnvs();
  });

  it("proxies summary request to backend", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const summaries = [
      { evalId: "tone", evalType: "llm_judge", total: 100, passed: 85, failed: 15, passRate: 85.0 },
    ];
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ summaries }),
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.summaries).toHaveLength(1);

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("/api/v1/eval-results/summary?");
    expect(fetchUrl).toContain("namespace=test-ws");
  });

  it("forwards filter query params", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ summaries: [] }),
    });

    const { GET } = await import("./route");
    await GET(
      createMockRequest("?agentName=myagent&createdAfter=2026-01-01T00:00:00Z"),
      createMockContext()
    );

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("agentName=myagent");
    expect(fetchUrl).toContain("createdAfter=");
  });

  it("returns 503 when SESSION_API_URL is not set", async () => {
    vi.stubEnv("SESSION_API_URL", "");
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(503);
  });

  it("returns 502 on fetch error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    mockFetch.mockRejectedValueOnce(new Error("Connection refused"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(502);
  });

  it("returns 403 when user lacks workspace access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: false,
      role: null,
      permissions: noPermissions,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(403);
  });
});

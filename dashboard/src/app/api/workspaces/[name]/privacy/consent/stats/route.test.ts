/**
 * Tests for the consent stats proxy route.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", () => ({
  getWorkspace: vi.fn(),
}));

vi.mock("@/lib/k8s/service-url-resolver", () => ({
  resolveServiceURLs: vi.fn(),
}));

const mockFetch = vi.fn();
global.fetch = mockFetch;

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

const mockWorkspace = {
  metadata: { uid: "workspace-uid-123", name: "test-ws" },
  spec: {},
};

function createMockContext() {
  return { params: Promise.resolve({ name: "test-ws" }) };
}

function createRequest(): NextRequest {
  return new NextRequest(
    `https://localhost:3000/api/workspaces/test-ws/privacy/consent/stats`,
    { method: "GET" },
  );
}

describe("GET /api/workspaces/[name]/privacy/consent/stats", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("proxies to memory-api consent stats endpoint", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);
    vi.mocked(resolveServiceURLs).mockResolvedValue({
      sessionURL: "https://session-api:8080",
      memoryURL: "https://memory-api:8080",
    });

    const stats = {
      totalUsers: 100,
      optedOutAll: 5,
      grantsByCategory: { "memory:context": 90 },
    };
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      text: () => Promise.resolve(JSON.stringify(stats)),
    });

    const { GET } = await import("./route");
    const response = await GET(createRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toEqual(stats);

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("/api/v1/privacy/consent/stats");
    expect(fetchUrl).toContain("workspace=workspace-uid-123");
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
    const response = await GET(createRequest(), createMockContext());

    expect(response.status).toBe(403);
  });
});

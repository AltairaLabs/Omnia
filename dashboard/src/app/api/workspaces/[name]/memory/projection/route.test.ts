/**
 * Tests for the memory projection proxy route.
 *
 * Covers:
 *  - happy path: proxies to the backend projection endpoint
 *  - workspace-wide scoping: only ?workspace=<uid> is sent (no user_id), so the
 *    galaxy is the institution-wide view, not the caller's own memories
 *  - status:"pending" passes through unchanged (the FE polls on it)
 *  - 403 when workspace access is denied
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
  return new NextRequest("https://localhost:3000/api/workspaces/test-ws/memory/projection", {
    method: "GET",
  });
}

function mockJsonResponse(data: unknown, status = 200) {
  mockFetch.mockResolvedValueOnce({
    ok: status >= 200 && status < 300,
    status,
    text: () => Promise.resolve(JSON.stringify(data)),
  });
}

async function authedGET() {
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
    namespace: "omnia-test", privacyURL: ""
  });

  const { GET } = await import("./route");
  return GET(createRequest(), createMockContext());
}

describe("GET /api/workspaces/[name]/memory/projection", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("proxies the workspace-wide projection to the backend", async () => {
    const layout = {
      model: "tsne",
      total: 3,
      capped: false,
      status: "ready",
      points: [{ id: "a", x: 0, y: 0, tier: "user", confidence: 0.9 }],
    };
    mockJsonResponse(layout);

    const response = await authedGET();
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toEqual(layout);

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("/api/v1/memories/projection");
    expect(fetchUrl).toContain("workspace=workspace-uid-123");
    // Workspace-wide: the caller's identity must NOT narrow the galaxy.
    expect(fetchUrl).not.toContain("user_id");
  });

  it("passes a pending status through unchanged", async () => {
    mockJsonResponse({ status: "pending", total: 5000, points: [] });

    const response = await authedGET();
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.status).toBe("pending");
    expect(body.total).toBe(5000);
    expect(body.points).toEqual([]);
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

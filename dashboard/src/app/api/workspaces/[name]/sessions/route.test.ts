/**
 * Tests for session list/search proxy route.
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

function createMockRequest(query = "", method = "GET"): NextRequest {
  const url = `http://localhost:3000/api/workspaces/test-ws/sessions${query}`;
  return new NextRequest(url, { method });
}

const ownerPermissions = { read: true, write: true, delete: true, manageMembers: true };

function createMockContext() {
  return { params: Promise.resolve({ name: "test-ws" }) };
}

describe("GET /api/workspaces/[name]/sessions", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("proxies sessions list to backend", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue({ spec: { namespace: { name: "test-ns" } } } as never);

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ sessions: [{ id: "s1" }], total: 1, hasMore: false }),
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.sessions).toHaveLength(1);

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("/api/v1/sessions?");
    expect(fetchUrl).toContain("namespace=test-ns");
    expect(fetchUrl).not.toContain("search");
  });

  it("routes to search endpoint when q param is present", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue({ spec: { namespace: { name: "test-ns" } } } as never);

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ sessions: [], total: 0, hasMore: false }),
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("?q=hello"), createMockContext());

    expect(response.status).toBe(200);

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("/api/v1/sessions/search?");
    expect(fetchUrl).toContain("q=hello");
  });

  it("forwards filter query params", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue({ spec: { namespace: { name: "test-ns" } } } as never);

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ sessions: [], total: 0, hasMore: false }),
    });

    const { GET } = await import("./route");
    await GET(createMockRequest("?agent=myagent&status=active&limit=10"), createMockContext());

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("agent=myagent");
    expect(fetchUrl).toContain("status=active");
    expect(fetchUrl).toContain("limit=10");
  });

  it("returns 503 when service URLs are not resolvable", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(null);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(503);
  });

  it("returns 502 on fetch error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue({ spec: { namespace: { name: "test-ns" } } } as never);

    mockFetch.mockRejectedValueOnce(new Error("Connection refused"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(502);
  });

  it("returns 404 when workspace not found", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(null);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns 403 when user lacks workspace access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: false, role: null, permissions: noPermissions });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(403);
  });
});

describe("DELETE /api/workspaces/[name]/sessions (bulk purge)", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("purges by namespace and returns the deleted count", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "owner", permissions: ownerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue({ spec: { namespace: { name: "test-ns" } } } as never);

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ deleted: 7 }),
    });

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("", "DELETE"), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.deleted).toBe(7);

    const call = mockFetch.mock.calls[0];
    expect(call[0]).toContain("/api/v1/sessions?");
    expect(call[0]).toContain("namespace=test-ns");
    expect(call[1]).toMatchObject({ method: "DELETE" });
  });

  it("forwards agent and before scope filters", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "owner", permissions: ownerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue({ spec: { namespace: { name: "test-ns" } } } as never);

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ deleted: 2 }),
    });

    const { DELETE } = await import("./route");
    await DELETE(
      createMockRequest("?agent=myagent&before=2026-01-01T00:00:00Z", "DELETE"),
      createMockContext()
    );

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("agent=myagent");
    expect(fetchUrl).toContain("before=2026-01-01");
  });

  it("returns 403 when user is not an owner", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: false, role: "editor", permissions: viewerPermissions });

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("", "DELETE"), createMockContext());

    expect(response.status).toBe(403);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("returns 503 when service URLs are not resolvable", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(null);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "owner", permissions: ownerPermissions });

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("", "DELETE"), createMockContext());

    expect(response.status).toBe(503);
  });

  it("returns 404 when workspace not found", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "owner", permissions: ownerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(null);

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("", "DELETE"), createMockContext());

    expect(response.status).toBe(404);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("returns 502 on backend connection error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "owner", permissions: ownerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue({ spec: { namespace: { name: "test-ns" } } } as never);

    mockFetch.mockRejectedValueOnce(new Error("Connection refused"));

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("", "DELETE"), createMockContext());

    expect(response.status).toBe(502);
  });
});

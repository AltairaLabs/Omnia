/**
 * Tests for session detail proxy route.
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

function createMockRequest(method = "GET"): NextRequest {
  return new NextRequest("http://localhost:3000/api/workspaces/test-ws/sessions/sess-123", { method });
}

const editorPermissions = { read: true, write: true, delete: true, manageMembers: false };

function createMockContext() {
  return { params: Promise.resolve({ name: "test-ws", sessionId: "sess-123" }) };
}

function mockWorkspace(namespace = "test-ns") {
  return {
    metadata: { name: "test-ws" },
    spec: { namespace: { name: namespace } },
  };
}

describe("GET /api/workspaces/[name]/sessions/[sessionId]", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("proxies session detail to backend", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace() as Awaited<ReturnType<typeof getWorkspace>>);

    // First fetch: namespace guard fetches session metadata
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ session: { id: "sess-123", namespace: "test-ns" } }),
    });
    // Second fetch: actual session detail request
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ session: { id: "sess-123" }, messages: [] }),
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.session.id).toBe("sess-123");

    const fetchUrl = mockFetch.mock.calls[1][0] as string;
    expect(fetchUrl).toContain("/api/v1/sessions/sess-123");
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
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace() as Awaited<ReturnType<typeof getWorkspace>>);

    // Namespace guard succeeds
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ session: { id: "sess-123", namespace: "test-ns" } }),
    });
    // Actual request fails
    mockFetch.mockRejectedValueOnce(new Error("Connection refused"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(502);
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

  it("forwards backend error status", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace() as Awaited<ReturnType<typeof getWorkspace>>);

    // Namespace guard: session not found at backend level
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
      json: () => Promise.resolve({ error: "Session not found" }),
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns 404 when session belongs to a different namespace (IDOR prevention)", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    // Workspace resolves to namespace-a
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace("namespace-a") as Awaited<ReturnType<typeof getWorkspace>>);

    // Session belongs to namespace-b
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ session: { id: "sess-123", namespace: "namespace-b" } }),
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.error).toBe("Session not found");

    // Should NOT make a second fetch for the actual data
    expect(mockFetch).toHaveBeenCalledTimes(1);
  });
});

describe("DELETE /api/workspaces/[name]/sessions/[sessionId]", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("deletes the session, scoping the backend call to the workspace namespace", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace() as Awaited<ReturnType<typeof getWorkspace>>);

    // Namespace guard fetch (session belongs to test-ns)
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ session: { id: "sess-123", namespace: "test-ns" } }),
    });
    // DELETE fetch → 204 No Content
    mockFetch.mockResolvedValueOnce({ ok: true, status: 204, json: () => Promise.resolve({}) });

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(204);
    const deleteCall = mockFetch.mock.calls[1];
    expect(deleteCall[0]).toContain("/api/v1/sessions/sess-123");
    expect(deleteCall[0]).toContain("namespace=test-ns");
    expect(deleteCall[1]).toMatchObject({ method: "DELETE" });
  });

  it("returns 403 when user lacks editor access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: false, role: "viewer", permissions: viewerPermissions });

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(403);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("returns 404 without issuing a delete when the session is in a foreign namespace", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace("namespace-a") as Awaited<ReturnType<typeof getWorkspace>>);

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ session: { id: "sess-123", namespace: "namespace-b" } }),
    });

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(404);
    // Only the guard fetch — never the delete
    expect(mockFetch).toHaveBeenCalledTimes(1);
  });

  it("forwards a non-204 backend response body", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace() as Awaited<ReturnType<typeof getWorkspace>>);

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ session: { id: "sess-123", namespace: "test-ns" } }),
    });
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      json: () => Promise.resolve({ error: "boom" }),
    });

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(500);
    const body = await response.json();
    expect(body.error).toBe("boom");
  });

  it("returns 502 on backend connection error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" });
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace() as Awaited<ReturnType<typeof getWorkspace>>);

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ session: { id: "sess-123", namespace: "test-ns" } }),
    });
    mockFetch.mockRejectedValueOnce(new Error("Connection refused"));

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(502);
  });
});

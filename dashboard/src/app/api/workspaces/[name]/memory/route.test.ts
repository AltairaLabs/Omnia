/**
 * Tests for memory API proxy routes.
 *
 * Covers:
 *  - proxy-helpers: resolveWorkspaceUID, buildBackendParams, proxyToMemoryApi
 *  - route.ts:       GET / DELETE handlers
 *  - search/route.ts: GET search handler
 *  - export/route.ts: GET export handler
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

// --- module mocks (hoisted before any dynamic imports) ---

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", () => ({
  getWorkspace: vi.fn(),
}));

// --- global fetch mock ---
const mockFetch = vi.fn();
global.fetch = mockFetch;

// --- shared fixtures ---

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

// --- helpers ---

function createMockContext() {
  return { params: Promise.resolve({ name: "test-ws" }) };
}

function createRequest(method: string, path: string, query = ""): NextRequest {
  const suffix = query ? "?" + query : "";
  return new NextRequest(
    `https://localhost:3000${path}${suffix}`,
    { method }
  );
}

function mockFetchJsonResponse(data: unknown, status = 200) {
  mockFetch.mockResolvedValueOnce({
    ok: status >= 200 && status < 300,
    status,
    text: () => Promise.resolve(JSON.stringify(data)),
  });
}

function mockFetchTextResponse(text: string, status: number) {
  mockFetch.mockResolvedValueOnce({
    ok: status >= 200 && status < 300,
    status,
    text: () => Promise.resolve(text),
  });
}

// --- proxy-helpers unit tests ---

describe("resolveWorkspaceUID", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
    vi.unstubAllEnvs();
  });

  it("returns UID from workspace metadata", async () => {
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

    const { resolveWorkspaceUID } = await import("./proxy-helpers");
    const uid = await resolveWorkspaceUID("test-ws");

    expect(uid).toBe("workspace-uid-123");
    expect(getWorkspace).toHaveBeenCalledWith("test-ws");
  });

  it("returns null when workspace not found", async () => {
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(null as never);

    const { resolveWorkspaceUID } = await import("./proxy-helpers");
    const uid = await resolveWorkspaceUID("missing-ws");

    expect(uid).toBeNull();
  });
});

describe("buildBackendParams", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("maps userId to user_id and sets workspace", async () => {
    const { buildBackendParams } = await import("./proxy-helpers");

    const searchParams = new URLSearchParams("userId=user-abc");
    const params = buildBackendParams(searchParams, "ws-uid-999");

    expect(params.get("workspace")).toBe("ws-uid-999");
    expect(params.get("user_id")).toBe("user-abc");
    expect(params.has("userId")).toBe(false);
  });

  it("forwards type, limit, and offset", async () => {
    const { buildBackendParams } = await import("./proxy-helpers");

    const searchParams = new URLSearchParams("type=fact&limit=20&offset=5");
    const params = buildBackendParams(searchParams, "ws-uid-999");

    expect(params.get("type")).toBe("fact");
    expect(params.get("limit")).toBe("20");
    expect(params.get("offset")).toBe("5");
  });
});

// --- proxyToMemoryApi unit tests ---

describe("proxyToMemoryApi", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
    vi.unstubAllEnvs();
  });

  it("returns 503 when MEMORY_API_URL is not set", async () => {
    vi.stubEnv("MEMORY_API_URL", "");

    const { proxyToMemoryApi } = await import("./proxy-helpers");
    const req = createRequest("GET", "/api/workspaces/test-ws/memory");
    const response = await proxyToMemoryApi(req, "test-ws", "/api/v1/memories");

    expect(response.status).toBe(503);
    const body = await response.json();
    expect(body.error).toContain("not configured");
  });

  it("returns 404 when workspace not found", async () => {
    vi.stubEnv("MEMORY_API_URL", "https://memory-api:8080");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(null as never);

    const { proxyToMemoryApi } = await import("./proxy-helpers");
    const req = createRequest("GET", "/api/workspaces/missing-ws/memory");
    const response = await proxyToMemoryApi(req, "missing-ws", "/api/v1/memories");

    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.error).toContain("Workspace not found");
  });

  it("returns 200 with proxied data on success", async () => {
    vi.stubEnv("MEMORY_API_URL", "https://memory-api:8080");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

    const payload = { memories: [{ id: "m1", content: "test memory" }], total: 1 };
    mockFetchJsonResponse(payload);

    const { proxyToMemoryApi } = await import("./proxy-helpers");
    const req = createRequest("GET", "/api/workspaces/test-ws/memory");
    const response = await proxyToMemoryApi(req, "test-ws", "/api/v1/memories");

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.memories).toHaveLength(1);
    expect(body.memories[0].id).toBe("m1");

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("https://memory-api:8080/api/v1/memories");
    expect(fetchUrl).toContain("workspace=workspace-uid-123");
  });

  it("returns 200 with empty list on backend 404 non-JSON response", async () => {
    vi.stubEnv("MEMORY_API_URL", "https://memory-api:8080");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

    mockFetchTextResponse("Not Found", 404);

    const { proxyToMemoryApi } = await import("./proxy-helpers");
    const req = createRequest("GET", "/api/workspaces/test-ws/memory");
    const response = await proxyToMemoryApi(req, "test-ws", "/api/v1/memories");

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.memories).toEqual([]);
    expect(body.total).toBe(0);
  });

  it("returns 502 on fetch error", async () => {
    vi.stubEnv("MEMORY_API_URL", "https://memory-api:8080");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

    mockFetch.mockRejectedValueOnce(new Error("Connection refused"));

    const { proxyToMemoryApi } = await import("./proxy-helpers");
    const req = createRequest("GET", "/api/workspaces/test-ws/memory");
    const response = await proxyToMemoryApi(req, "test-ws", "/api/v1/memories");

    expect(response.status).toBe(502);
    const body = await response.json();
    expect(body.error).toContain("Failed to connect");
  });

  it("returns 502 on backend non-JSON non-404 response", async () => {
    vi.stubEnv("MEMORY_API_URL", "https://memory-api:8080");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

    mockFetchTextResponse("Internal Server Error", 500);

    const { proxyToMemoryApi } = await import("./proxy-helpers");
    const req = createRequest("GET", "/api/workspaces/test-ws/memory");
    const response = await proxyToMemoryApi(req, "test-ws", "/api/v1/memories");

    expect(response.status).toBe(502);
    const body = await response.json();
    expect(body.error).toContain("non-JSON");
  });

  it("strips trailing slash from MEMORY_API_URL", async () => {
    vi.stubEnv("MEMORY_API_URL", "https://memory-api:8080/");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

    mockFetchJsonResponse({ memories: [], total: 0 });

    const { proxyToMemoryApi } = await import("./proxy-helpers");
    const req = createRequest("GET", "/api/workspaces/test-ws/memory");
    await proxyToMemoryApi(req, "test-ws", "/api/v1/memories");

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).not.toContain("//api");
    expect(fetchUrl).toContain("https://memory-api:8080/api/v1/memories");
  });
});

// --- GET /api/workspaces/[name]/memory ---

describe("GET /api/workspaces/[name]/memory", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("MEMORY_API_URL", "https://memory-api:8080");
  });

  afterEach(() => {
    vi.resetAllMocks();
    vi.unstubAllEnvs();
  });

  it("returns memories list from backend", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

    const payload = { memories: [{ id: "m1" }, { id: "m2" }], total: 2 };
    mockFetchJsonResponse(payload);

    const { GET } = await import("./route");
    const req = createRequest("GET", "/api/workspaces/test-ws/memory");
    const response = await GET(req, createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.total).toBe(2);

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("/api/v1/memories");
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
    const req = createRequest("GET", "/api/workspaces/test-ws/memory");
    const response = await GET(req, createMockContext());

    expect(response.status).toBe(403);
  });
});

// --- DELETE /api/workspaces/[name]/memory ---

describe("DELETE /api/workspaces/[name]/memory", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("MEMORY_API_URL", "https://memory-api:8080");
  });

  afterEach(() => {
    vi.resetAllMocks();
    vi.unstubAllEnvs();
  });

  it("proxies DELETE to backend", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

    mockFetchJsonResponse({ deleted: 3 });

    const { DELETE } = await import("./route");
    const req = createRequest("DELETE", "/api/workspaces/test-ws/memory", "userId=user-abc");
    const response = await DELETE(req, createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.deleted).toBe(3);

    const [fetchUrl, fetchOpts] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(fetchUrl).toContain("/api/v1/memories");
    expect(fetchOpts.method).toBe("DELETE");
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

    const { DELETE } = await import("./route");
    const req = createRequest("DELETE", "/api/workspaces/test-ws/memory");
    const response = await DELETE(req, createMockContext());

    expect(response.status).toBe(403);
  });
});

// --- GET /api/workspaces/[name]/memory/search ---

describe("GET /api/workspaces/[name]/memory/search", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("MEMORY_API_URL", "https://memory-api:8080");
  });

  afterEach(() => {
    vi.resetAllMocks();
    vi.unstubAllEnvs();
  });

  it("adds q and min_confidence params to backend request", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

    const payload = { memories: [{ id: "m3", content: "relevant memory" }], total: 1 };
    mockFetchJsonResponse(payload);

    const { GET } = await import("./search/route");
    const req = createRequest(
      "GET",
      "/api/workspaces/test-ws/memory/search",
      "query=relevant+stuff&minConfidence=0.8"
    );
    const response = await GET(req, createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.memories).toHaveLength(1);

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("/api/v1/memories/search");
    expect(fetchUrl).toContain("q=relevant");
    expect(fetchUrl).toContain("min_confidence=0.8");
    expect(fetchUrl).toContain("workspace=workspace-uid-123");
  });

  it("accepts q param directly (alternative to query)", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

    mockFetchJsonResponse({ memories: [], total: 0 });

    const { GET } = await import("./search/route");
    const req = createRequest(
      "GET",
      "/api/workspaces/test-ws/memory/search",
      "q=hello"
    );
    const response = await GET(req, createMockContext());

    expect(response.status).toBe(200);
    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("q=hello");
  });

  it("returns 404 when workspace not found", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    vi.mocked(getWorkspace).mockResolvedValue(null as never);

    const { GET } = await import("./search/route");
    const req = createRequest("GET", "/api/workspaces/test-ws/memory/search", "q=hello");
    const response = await GET(req, createMockContext());

    expect(response.status).toBe(404);
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

    const { GET } = await import("./search/route");
    const req = createRequest("GET", "/api/workspaces/test-ws/memory/search");
    const response = await GET(req, createMockContext());

    expect(response.status).toBe(403);
  });
});

// --- DELETE /api/workspaces/[name]/memory/[memoryId] ---

describe("DELETE /api/workspaces/[name]/memory/[memoryId]", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("MEMORY_API_URL", "https://memory-api:8080");
  });

  afterEach(() => {
    vi.resetAllMocks();
    vi.unstubAllEnvs();
  });

  function createMemoryIdContext(memoryId = "mem-456") {
    return { params: Promise.resolve({ name: "test-ws", memoryId }) };
  }

  it("deletes a specific memory by ID", async () => {
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

    mockFetch.mockResolvedValueOnce({ ok: true, status: 200 });

    const { DELETE } = await import("./[memoryId]/route");
    const req = createRequest("DELETE", "/api/workspaces/test-ws/memory/mem-456");
    const response = await DELETE(req, createMemoryIdContext());

    expect(response.status).toBe(200);

    const [fetchUrl, fetchOpts] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(fetchUrl).toContain("/api/v1/memories/mem-456");
    expect(fetchUrl).toContain("workspace=workspace-uid-123");
    expect(fetchOpts.method).toBe("DELETE");
  });

  it("returns 503 when MEMORY_API_URL is not configured", async () => {
    vi.stubEnv("MEMORY_API_URL", "");

    const { DELETE } = await import("./[memoryId]/route");
    const req = createRequest("DELETE", "/api/workspaces/test-ws/memory/mem-456");
    const response = await DELETE(req, createMemoryIdContext());

    expect(response.status).toBe(503);
  });

  it("returns 404 when workspace not found", async () => {
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(null as never);

    const { DELETE } = await import("./[memoryId]/route");
    const req = createRequest("DELETE", "/api/workspaces/test-ws/memory/mem-456");
    const response = await DELETE(req, createMemoryIdContext());

    expect(response.status).toBe(404);
  });

  it("forwards non-ok backend status", async () => {
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 409,
      json: () => Promise.resolve({ error: "conflict" }),
    });

    const { DELETE } = await import("./[memoryId]/route");
    const req = createRequest("DELETE", "/api/workspaces/test-ws/memory/mem-456");
    const response = await DELETE(req, createMemoryIdContext());

    expect(response.status).toBe(409);
  });

  it("returns 502 on fetch error", async () => {
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

    mockFetch.mockRejectedValueOnce(new Error("ECONNREFUSED"));

    const { DELETE } = await import("./[memoryId]/route");
    const req = createRequest("DELETE", "/api/workspaces/test-ws/memory/mem-456");
    const response = await DELETE(req, createMemoryIdContext());

    expect(response.status).toBe(502);
  });

  it("handles non-JSON error body from backend gracefully", async () => {
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      json: () => Promise.reject(new Error("not JSON")),
    });

    const { DELETE } = await import("./[memoryId]/route");
    const req = createRequest("DELETE", "/api/workspaces/test-ws/memory/mem-456");
    const response = await DELETE(req, createMemoryIdContext());

    expect(response.status).toBe(500);
    const body = await response.json();
    expect(body.error).toBe("Delete failed");
  });
});

// --- GET /api/workspaces/[name]/memory/export ---

describe("GET /api/workspaces/[name]/memory/export", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("MEMORY_API_URL", "https://memory-api:8080");
  });

  afterEach(() => {
    vi.resetAllMocks();
    vi.unstubAllEnvs();
  });

  it("proxies to export endpoint", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);

    const exportData = {
      user_id: "user-abc",
      workspace: "workspace-uid-123",
      memories: [{ id: "m1", content: "exported" }],
      exported_at: "2026-04-02T00:00:00Z",
    };
    mockFetchJsonResponse(exportData);

    const { GET } = await import("./export/route");
    const req = createRequest(
      "GET",
      "/api/workspaces/test-ws/memory/export",
      "userId=user-abc"
    );
    const response = await GET(req, createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.memories).toHaveLength(1);

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("/api/v1/memories/export");
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

    const { GET } = await import("./export/route");
    const req = createRequest("GET", "/api/workspaces/test-ws/memory/export");
    const response = await GET(req, createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 503 when MEMORY_API_URL is not configured", async () => {
    vi.stubEnv("MEMORY_API_URL", "");
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const { GET } = await import("./export/route");
    const req = createRequest("GET", "/api/workspaces/test-ws/memory/export");
    const response = await GET(req, createMockContext());

    expect(response.status).toBe(503);
  });
});

/**
 * Tests for institutional memory proxy routes (list + create + delete).
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
  role: "editor" as const,
};
const viewerPerms = { read: true, write: false, delete: false, manageMembers: false };
const editorPerms = { read: true, write: true, delete: true, manageMembers: false };
const noPerms = { read: false, write: false, delete: false, manageMembers: false };

const mockWorkspace = { metadata: { uid: "ws-uid-42", name: "my-ws" }, spec: {} };

function ctx(overrides: Record<string, string> = {}) {
  return { params: Promise.resolve({ name: "my-ws", ...overrides }) };
}

function createReq(method: string, path: string, body?: unknown): NextRequest {
  if (body === undefined) {
    return new NextRequest(`https://localhost:3000${path}`, { method });
  }
  const bodyStr = typeof body === "string" ? body : JSON.stringify(body);
  return new NextRequest(`https://localhost:3000${path}`, {
    method,
    body: bodyStr,
    headers: { "Content-Type": "application/json" },
  });
}

function mockResp(data: unknown, status = 200) {
  mockFetch.mockResolvedValueOnce({
    ok: status >= 200 && status < 300,
    status,
    text: () => Promise.resolve(JSON.stringify(data)),
    json: () => Promise.resolve(data),
  });
}

describe("GET /api/workspaces/[name]/institutional-memory", () => {
  beforeEach(() => { vi.resetModules(); });
  afterEach(() => { vi.resetAllMocks(); });

  it("returns the memory-api list response", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPerms });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);
    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://s:8080", memoryURL: "https://m:8080" });

    mockResp({ memories: [{ id: "inst-1" }], total: 1 });

    const { GET } = await import("./route");
    const res = await GET(createReq("GET", "/api/workspaces/my-ws/institutional-memory?limit=10&offset=5"), ctx());

    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.memories).toHaveLength(1);

    const url = mockFetch.mock.calls[0][0] as string;
    expect(url).toContain("/api/v1/institutional/memories");
    expect(url).toContain("workspace=ws-uid-42");
    expect(url).toContain("limit=10");
    expect(url).toContain("offset=5");
  });

  it("returns 503 when memoryURL is not configured", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPerms });
    vi.mocked(resolveServiceURLs).mockResolvedValue(null);

    const { GET } = await import("./route");
    const res = await GET(createReq("GET", "/api/workspaces/my-ws/institutional-memory"), ctx());
    expect(res.status).toBe(503);
  });

  it("returns 404 when workspace not found", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPerms });
    vi.mocked(getWorkspace).mockResolvedValue(null as never);
    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://s:8080", memoryURL: "https://m:8080" });

    const { GET } = await import("./route");
    const res = await GET(createReq("GET", "/api/workspaces/my-ws/institutional-memory"), ctx());
    expect(res.status).toBe(404);
  });

  it("returns 502 on non-JSON backend response", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPerms });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);
    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://s:8080", memoryURL: "https://m:8080" });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      text: () => Promise.resolve("plain text"),
    });

    const { GET } = await import("./route");
    const res = await GET(createReq("GET", "/api/workspaces/my-ws/institutional-memory"), ctx());
    expect(res.status).toBe(502);
  });

  it("returns 502 on backend network error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPerms });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);
    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://s:8080", memoryURL: "https://m:8080" });

    mockFetch.mockRejectedValueOnce(new Error("net down"));

    const { GET } = await import("./route");
    const res = await GET(createReq("GET", "/api/workspaces/my-ws/institutional-memory"), ctx());
    expect(res.status).toBe(502);
  });

  it("returns 403 when viewer access is denied", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: false, role: null, permissions: noPerms });

    const { GET } = await import("./route");
    const res = await GET(createReq("GET", "/api/workspaces/my-ws/institutional-memory"), ctx());
    expect(res.status).toBe(403);
  });
});

describe("POST /api/workspaces/[name]/institutional-memory", () => {
  beforeEach(() => { vi.resetModules(); });
  afterEach(() => { vi.resetAllMocks(); });

  it("forwards body with workspace_id substituted to the UID", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPerms });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);
    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://s:8080", memoryURL: "https://m:8080" });

    mockResp({ memory: { id: "inst-new", type: "policy", content: "c", confidence: 1, scope: {}, createdAt: "" } }, 201);

    const { POST } = await import("./route");
    const res = await POST(
      createReq("POST", "/api/workspaces/my-ws/institutional-memory", {
        workspace_id: "attacker-forged-uid",
        type: "policy",
        content: "c",
      }),
      ctx()
    );

    expect(res.status).toBe(201);

    const requestInit = mockFetch.mock.calls[0][1] as RequestInit;
    const forwardedBody = JSON.parse(requestInit.body as string);
    expect(forwardedBody.workspace_id).toBe("ws-uid-42");
    expect(forwardedBody.type).toBe("policy");
  });

  it("rejects editor-missing with 403", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: false, role: null, permissions: viewerPerms });

    const { POST } = await import("./route");
    const res = await POST(createReq("POST", "/api/workspaces/my-ws/institutional-memory", { type: "t", content: "c" }), ctx());
    expect(res.status).toBe(403);
  });

  it("returns 400 on invalid JSON body", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPerms });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);
    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://s:8080", memoryURL: "https://m:8080" });

    const { POST } = await import("./route");
    const res = await POST(createReq("POST", "/api/workspaces/my-ws/institutional-memory", "not json"), ctx());
    expect(res.status).toBe(400);
  });

  it("returns 503 when memoryURL missing", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPerms });
    vi.mocked(resolveServiceURLs).mockResolvedValue(null);

    const { POST } = await import("./route");
    const res = await POST(createReq("POST", "/api/workspaces/my-ws/institutional-memory", { type: "t", content: "c" }), ctx());
    expect(res.status).toBe(503);
  });

  it("returns 404 when workspace missing", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPerms });
    vi.mocked(getWorkspace).mockResolvedValue(null as never);
    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://s:8080", memoryURL: "https://m:8080" });

    const { POST } = await import("./route");
    const res = await POST(createReq("POST", "/api/workspaces/my-ws/institutional-memory", { type: "t", content: "c" }), ctx());
    expect(res.status).toBe(404);
  });

  it("returns 502 on backend network error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPerms });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);
    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://s:8080", memoryURL: "https://m:8080" });

    mockFetch.mockRejectedValueOnce(new Error("boom"));

    const { POST } = await import("./route");
    const res = await POST(createReq("POST", "/api/workspaces/my-ws/institutional-memory", { type: "t", content: "c" }), ctx());
    expect(res.status).toBe(502);
  });
});

describe("DELETE /api/workspaces/[name]/institutional-memory/[memoryId]", () => {
  beforeEach(() => { vi.resetModules(); });
  afterEach(() => { vi.resetAllMocks(); });

  function deleteCtx() {
    return { params: Promise.resolve({ name: "my-ws", memoryId: "inst-1" }) };
  }

  it("deletes the memory and returns 200", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPerms });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);
    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://s:8080", memoryURL: "https://m:8080" });

    mockFetch.mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve({}) });

    const { DELETE } = await import("./[memoryId]/route");
    const res = await DELETE(createReq("DELETE", "/api/workspaces/my-ws/institutional-memory/inst-1"), deleteCtx());
    expect(res.status).toBe(200);

    const url = mockFetch.mock.calls[0][0] as string;
    expect(url).toContain("/api/v1/institutional/memories/inst-1");
    expect(url).toContain("workspace=ws-uid-42");
  });

  it("forwards backend 400 (ErrNotInstitutional)", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPerms });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);
    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://s:8080", memoryURL: "https://m:8080" });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 400,
      json: () => Promise.resolve({ error: "memory: target is not an institutional memory" }),
    });

    const { DELETE } = await import("./[memoryId]/route");
    const res = await DELETE(createReq("DELETE", "/api/workspaces/my-ws/institutional-memory/inst-1"), deleteCtx());
    expect(res.status).toBe(400);
    const body = await res.json();
    expect(body.error).toContain("not an institutional");
  });

  it("returns 503 when service URL not configured", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPerms });
    vi.mocked(resolveServiceURLs).mockResolvedValue(null);

    const { DELETE } = await import("./[memoryId]/route");
    const res = await DELETE(createReq("DELETE", "/api/workspaces/my-ws/institutional-memory/inst-1"), deleteCtx());
    expect(res.status).toBe(503);
  });

  it("returns 404 when workspace missing", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPerms });
    vi.mocked(getWorkspace).mockResolvedValue(null as never);
    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://s:8080", memoryURL: "https://m:8080" });

    const { DELETE } = await import("./[memoryId]/route");
    const res = await DELETE(createReq("DELETE", "/api/workspaces/my-ws/institutional-memory/inst-1"), deleteCtx());
    expect(res.status).toBe(404);
  });

  it("returns 502 on backend network error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPerms });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);
    vi.mocked(resolveServiceURLs).mockResolvedValue({ sessionURL: "https://s:8080", memoryURL: "https://m:8080" });

    mockFetch.mockRejectedValueOnce(new Error("net"));

    const { DELETE } = await import("./[memoryId]/route");
    const res = await DELETE(createReq("DELETE", "/api/workspaces/my-ws/institutional-memory/inst-1"), deleteCtx());
    expect(res.status).toBe(502);
  });
});

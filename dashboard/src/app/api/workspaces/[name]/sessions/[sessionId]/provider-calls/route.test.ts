/**
 * Tests for session provider-calls proxy route.
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

function createMockRequest(): NextRequest {
  return new NextRequest(
    "http://localhost:3000/api/workspaces/test-ws/sessions/sess-123/provider-calls",
    { method: "GET" }
  );
}

function createMockContext() {
  return { params: Promise.resolve({ name: "test-ws", sessionId: "sess-123" }) };
}

function mockWorkspace(namespace = "test-ns") {
  return {
    metadata: { name: "test-ws" },
    spec: { namespace: { name: namespace } },
  };
}

describe("GET /api/workspaces/[name]/sessions/[sessionId]/provider-calls", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("SESSION_API_URL", "https://session-api:8080");
  });

  afterEach(() => {
    vi.resetAllMocks();
    vi.unstubAllEnvs();
  });

  it("proxies provider-calls request to backend", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace() as Awaited<ReturnType<typeof getWorkspace>>);

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ session: { id: "sess-123", namespace: "test-ns" } }),
    });
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ providerCalls: [{ id: "pc1", model: "gpt-4" }] }),
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.providerCalls).toHaveLength(1);

    const fetchUrl = mockFetch.mock.calls[1][0] as string;
    expect(fetchUrl).toContain("/api/v1/sessions/sess-123/provider-calls");
  });

  it("returns 404 when session belongs to a different namespace (IDOR prevention)", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace("namespace-a") as Awaited<ReturnType<typeof getWorkspace>>);

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
    expect(mockFetch).toHaveBeenCalledTimes(1);
  });

  it("returns 503 when SESSION_API_URL is not set", async () => {
    vi.stubEnv("SESSION_API_URL", "");
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(503);
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

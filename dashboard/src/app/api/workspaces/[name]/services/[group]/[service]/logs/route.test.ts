/**
 * Tests for the per-service logs API route.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/workspace-route-helpers")>();
  return {
    ...actual,
    validateWorkspace: vi.fn(),
  };
});

vi.mock("@/lib/k8s/crd-operations", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/crd-operations")>();
  return {
    ...actual,
    getPodLogs: vi.fn(),
  };
});

vi.mock("@/lib/audit", () => ({
  logError: vi.fn(),
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

const mockLogs = [{ pod: "memory-api-0", container: "memory-api", timestamp: "2026-07-03T00:00:00Z", message: "hello" }];

function createMockRequest(): NextRequest {
  return new NextRequest("http://localhost:3000/api/workspaces/test-ws/services/default/memory-api/logs", {
    method: "GET",
  });
}

function createMockContext(group: string, service: string) {
  return { params: Promise.resolve({ name: "test-ws", group, service }) };
}

describe("GET /api/workspaces/[name]/services/[group]/[service]/logs", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("builds a group-scoped selector and returns 200 with logs", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getPodLogs } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: { metadata: { name: "test-ws" }, spec: { namespace: { name: "test-ns" } } } as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });
    vi.mocked(getPodLogs).mockResolvedValue(mockLogs as any);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext("default", "memory-api"));

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toEqual({ logs: mockLogs });
    expect(getPodLogs).toHaveBeenCalledWith(
      { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      "app.kubernetes.io/component=memory-api,omnia.altairalabs.ai/service-group=default",
      100,
      undefined
    );
  });

  it("builds a workspace-level selector (no group clause) for __workspace__ group", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getPodLogs } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: { metadata: { name: "test-ws" }, spec: { namespace: { name: "test-ns" } } } as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });
    vi.mocked(getPodLogs).mockResolvedValue([] as any);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext("__workspace__", "privacy-api"));

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toEqual({ logs: [] });
    expect(getPodLogs).toHaveBeenCalledWith(
      { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      "app.kubernetes.io/component=privacy-api",
      100,
      undefined
    );
  });

  it("returns 404 when the workspace does not exist", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getPodLogs } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: false,
      response: new Response(JSON.stringify({ error: "Not Found" }), { status: 404 }) as any,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext("default", "memory-api"));

    expect(response.status).toBe(404);
    expect(getPodLogs).not.toHaveBeenCalled();
  });

  it("returns 500 when getPodLogs throws", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getPodLogs } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: { metadata: { name: "test-ws" }, spec: { namespace: { name: "test-ns" } } } as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });
    vi.mocked(getPodLogs).mockRejectedValue(new Error("K8s connection failed"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext("default", "memory-api"));

    expect(response.status).toBe(500);
    const body = await response.json();
    expect(body.error).toBe("Internal Server Error");
  });
});

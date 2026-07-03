/**
 * Tests for the workspace service-health API route.
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

vi.mock("@/lib/k8s/service-health", () => ({
  getServiceHealth: vi.fn(),
}));

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
const noPermissions = { read: false, write: false, delete: false, manageMembers: false };

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

const mockHealth = {
  workspaceServices: [{ service: "privacy-api", state: "ready", ready: true, restarts: 0 }],
  groups: [{ name: "default", ready: true, members: [] }],
  source: "crd" as const,
};

function createMockRequest(): NextRequest {
  return new NextRequest("http://localhost:3000/api/workspaces/test-ws/services", { method: "GET" });
}

function createMockContext() {
  return { params: Promise.resolve({ name: "test-ws" }) };
}

describe("GET /api/workspaces/[name]/services", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns assembled service health for an authorized viewer", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getServiceHealth } = await import("@/lib/k8s/service-health");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });
    vi.mocked(getServiceHealth).mockResolvedValue(mockHealth as any);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toEqual(mockHealth);
    expect(getServiceHealth).toHaveBeenCalledWith(
      { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      "test-ws"
    );
  });

  it("returns 403 when user lacks workspace access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getServiceHealth } = await import("@/lib/k8s/service-health");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: false, role: null, permissions: noPermissions });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(403);
    expect(getServiceHealth).not.toHaveBeenCalled();
  });

  it("returns 404 when the workspace does not exist", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getServiceHealth } = await import("@/lib/k8s/service-health");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: false,
      response: new Response(JSON.stringify({ error: "Not Found" }), { status: 404 }) as any,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
    expect(getServiceHealth).not.toHaveBeenCalled();
  });

  it("returns 500 when getServiceHealth throws", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getServiceHealth } = await import("@/lib/k8s/service-health");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });
    vi.mocked(getServiceHealth).mockRejectedValue(new Error("K8s connection failed"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(500);
    const body = await response.json();
    expect(body.error).toBe("Internal Server Error");
  });
});

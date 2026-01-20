/**
 * Tests for Arena source sync API route.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

// Mock dependencies before imports
vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/k8s/crd-operations", () => ({
  patchCrd: vi.fn(),
  getCrd: vi.fn(),
  extractK8sErrorMessage: vi.fn((error: unknown) =>
    error instanceof Error ? error.message : String(error)
  ),
  isForbiddenError: vi.fn(() => false),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/workspace-route-helpers")>();
  return {
    ...actual,
    getWorkspaceResource: vi.fn(),
    createAuditContext: vi.fn(() => ({
      workspace: "test-ws",
      namespace: "test-ns",
      user: { username: "testuser" },
      role: "editor",
      resourceType: "ArenaSource",
    })),
    auditSuccess: vi.fn(),
    auditError: vi.fn(),
  };
});

vi.mock("@/lib/audit", () => ({
  logError: vi.fn(),
  logCrdSuccess: vi.fn(),
  logCrdDenied: vi.fn(),
  logCrdError: vi.fn(),
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
const editorPermissions = { read: true, write: true, delete: true, manageMembers: false };

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

const mockSource = {
  metadata: { name: "git-source", namespace: "test-ns" },
  spec: { type: "git", git: { url: "https://github.com/example/repo" } },
  status: { phase: "Ready" },
};

function createMockRequest(): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/arena/sources/git-source/sync";
  return new NextRequest(url, { method: "POST" });
}

function createMockContext() {
  return {
    params: Promise.resolve({ name: "test-ws", sourceName: "git-source" }),
  };
}

describe("POST /api/workspaces/[name]/arena/sources/[sourceName]/sync", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("triggers sync for user with editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { patchCrd } = await import("@/lib/k8s/crd-operations");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: mockSource,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    });
    vi.mocked(patchCrd).mockResolvedValue(mockSource);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.message).toBe("Sync triggered");
    expect(body.sourceName).toBe("git-source");
    expect(patchCrd).toHaveBeenCalledWith(
      expect.any(Object),
      "arenasources",
      "git-source",
      expect.objectContaining({
        metadata: expect.objectContaining({
          annotations: expect.objectContaining({
            "omnia.altairalabs.ai/reconcile-at": expect.any(String),
          }),
        }),
      })
    );
  });

  it("returns 404 when source not found", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource, notFoundResponse } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: false,
      response: notFoundResponse("Arena source not found: git-source"),
    });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns 403 when user lacks editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: false, role: "viewer", permissions: viewerPermissions });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest(), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 500 on patch error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { patchCrd } = await import("@/lib/k8s/crd-operations");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: mockSource,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    });
    vi.mocked(patchCrd).mockRejectedValue(new Error("Patch failed"));

    const { POST } = await import("./route");
    const response = await POST(createMockRequest(), createMockContext());

    expect(response.status).toBe(500);
  });
});

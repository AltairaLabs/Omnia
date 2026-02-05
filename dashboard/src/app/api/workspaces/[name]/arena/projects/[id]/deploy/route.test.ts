/**
 * Tests for Arena project deploy API routes.
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

vi.mock("@/lib/k8s/workspace-route-helpers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/workspace-route-helpers")>();
  return {
    ...actual,
    getWorkspace: vi.fn(),
    validateWorkspace: vi.fn(),
    createAuditContext: vi.fn().mockReturnValue({}),
    auditSuccess: vi.fn(),
    auditError: vi.fn(),
  };
});

vi.mock("@/lib/k8s/crd-operations", () => ({
  getCrd: vi.fn(),
  createCrd: vi.fn(),
  updateCrd: vi.fn(),
}));

vi.mock("@/lib/k8s/workspace-k8s-client-factory", () => ({
  getWorkspaceCoreApi: vi.fn(),
  withTokenRefresh: vi.fn((opts, fn) => fn()),
}));

vi.mock("node:fs/promises", () => ({
  access: vi.fn(),
  readdir: vi.fn(),
  readFile: vi.fn(),
}));

const mockUser = {
  id: "testuser-id",
  provider: "oauth" as const,
  username: "testuser",
  email: "test@example.com",
  groups: ["users"],
  role: "editor" as const,
};

const editorPermissions = { read: true, write: true, delete: true, manageMembers: false };
const noPermissions = { read: false, write: false, delete: false, manageMembers: false };

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

function createMockRequest(body?: object): NextRequest {
  const url = new URL("http://localhost:3000/api/workspaces/test-ws/arena/projects/project-1/deploy");
  const req = new NextRequest(url.toString(), {
    method: "POST",
    body: body ? JSON.stringify(body) : undefined,
    headers: body ? { "Content-Type": "application/json" } : undefined,
  });
  return req;
}

function createMockContext(projectId = "project-1") {
  return {
    params: Promise.resolve({ name: "test-ws", id: projectId }),
  };
}

describe("POST /api/workspaces/[name]/arena/projects/[id]/deploy", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns 403 when user lacks editor access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: false, role: null, permissions: noPermissions });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest(), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 404 when project does not exist", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.access).mockRejectedValue(new Error("ENOENT"));

    const { POST } = await import("./route");
    const response = await POST(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns 400 when project has no files", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.access).mockResolvedValue(undefined);
    vi.mocked(fs.readdir).mockResolvedValue([]);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest(), createMockContext());

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("no files");
  });

  it("creates new ArenaSource when deploying for first time", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd, createCrd } = await import("@/lib/k8s/crd-operations");
    const { getWorkspaceCoreApi } = await import("@/lib/k8s/workspace-k8s-client-factory");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.access).mockResolvedValue(undefined);
    vi.mocked(fs.readdir).mockResolvedValue([
      { name: "config.yaml", isDirectory: () => false },
    ] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(fs.readFile).mockResolvedValue("content: test");
    vi.mocked(getCrd).mockResolvedValue(null);
    vi.mocked(createCrd).mockResolvedValue({
      metadata: { name: "project-project-1", namespace: "test-ns" },
      spec: { type: "configmap", configMap: { name: "arena-project-project-1" } },
    });
    vi.mocked(getWorkspaceCoreApi).mockResolvedValue({
      readNamespacedConfigMap: vi.fn().mockRejectedValue({ statusCode: 404 }),
      createNamespacedConfigMap: vi.fn().mockResolvedValue({}),
    } as unknown as Awaited<ReturnType<typeof getWorkspaceCoreApi>>);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest(), createMockContext());

    expect(response.status).toBe(201);
    const body = await response.json();
    expect(body.isNew).toBe(true);
    expect(body.configMap.name).toBe("arena-project-project-1");
  });

  it("updates existing ArenaSource when redeploying", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd, updateCrd } = await import("@/lib/k8s/crd-operations");
    const { getWorkspaceCoreApi } = await import("@/lib/k8s/workspace-k8s-client-factory");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.access).mockResolvedValue(undefined);
    vi.mocked(fs.readdir).mockResolvedValue([
      { name: "config.yaml", isDirectory: () => false },
    ] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(fs.readFile).mockResolvedValue("content: updated");
    vi.mocked(getCrd).mockResolvedValue({
      metadata: { name: "project-project-1", namespace: "test-ns", labels: {} },
      spec: { type: "configmap", configMap: { name: "arena-project-project-1" }, interval: "5m" },
    });
    vi.mocked(updateCrd).mockResolvedValue({
      metadata: { name: "project-project-1", namespace: "test-ns" },
      spec: { type: "configmap", configMap: { name: "arena-project-project-1" } },
    });
    vi.mocked(getWorkspaceCoreApi).mockResolvedValue({
      readNamespacedConfigMap: vi.fn().mockResolvedValue({}),
      replaceNamespacedConfigMap: vi.fn().mockResolvedValue({}),
    } as unknown as Awaited<ReturnType<typeof getWorkspaceCoreApi>>);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.isNew).toBe(false);
  });

  it("reads files from subdirectories recursively", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd, createCrd } = await import("@/lib/k8s/crd-operations");
    const { getWorkspaceCoreApi } = await import("@/lib/k8s/workspace-k8s-client-factory");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.access).mockResolvedValue(undefined);
    vi.mocked(fs.readdir)
      .mockResolvedValueOnce([
        { name: "config.yaml", isDirectory: () => false },
        { name: "prompts", isDirectory: () => true },
        { name: ".hidden", isDirectory: () => true }, // Should be skipped
      ] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>)
      .mockResolvedValueOnce([
        { name: "main.yaml", isDirectory: () => false },
      ] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(fs.readFile).mockResolvedValue("content: test");
    vi.mocked(getCrd).mockResolvedValue(null);
    vi.mocked(createCrd).mockResolvedValue({
      metadata: { name: "project-project-1", namespace: "test-ns" },
      spec: { type: "configmap", configMap: { name: "arena-project-project-1" } },
    });

    const mockCoreApi = {
      readNamespacedConfigMap: vi.fn().mockRejectedValue({ statusCode: 404 }),
      createNamespacedConfigMap: vi.fn().mockResolvedValue({}),
    };
    vi.mocked(getWorkspaceCoreApi).mockResolvedValue(mockCoreApi as unknown as Awaited<ReturnType<typeof getWorkspaceCoreApi>>);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest(), createMockContext());

    expect(response.status).toBe(201);
    // Verify ConfigMap was created with encoded paths
    expect(mockCoreApi.createNamespacedConfigMap).toHaveBeenCalledWith(
      expect.objectContaining({
        body: expect.objectContaining({
          data: expect.objectContaining({
            "config.yaml": "content: test",
            "prompts__main.yaml": "content: test",
          }),
        }),
      })
    );
  });

  it("uses custom source name from request body", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { getCrd, createCrd } = await import("@/lib/k8s/crd-operations");
    const { getWorkspaceCoreApi } = await import("@/lib/k8s/workspace-k8s-client-factory");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.access).mockResolvedValue(undefined);
    vi.mocked(fs.readdir).mockResolvedValue([
      { name: "config.yaml", isDirectory: () => false },
    ] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(fs.readFile).mockResolvedValue("content: test");
    vi.mocked(getCrd).mockResolvedValue(null);
    vi.mocked(createCrd).mockResolvedValue({
      metadata: { name: "custom-source-name", namespace: "test-ns" },
      spec: { type: "configmap", configMap: { name: "arena-project-project-1" } },
    });
    vi.mocked(getWorkspaceCoreApi).mockResolvedValue({
      readNamespacedConfigMap: vi.fn().mockRejectedValue({ statusCode: 404 }),
      createNamespacedConfigMap: vi.fn().mockResolvedValue({}),
    } as unknown as Awaited<ReturnType<typeof getWorkspaceCoreApi>>);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ name: "custom-source-name", syncInterval: "10m" }), createMockContext());

    expect(response.status).toBe(201);
    expect(getCrd).toHaveBeenCalledWith(
      expect.anything(),
      expect.anything(),
      "custom-source-name"
    );
  });
});

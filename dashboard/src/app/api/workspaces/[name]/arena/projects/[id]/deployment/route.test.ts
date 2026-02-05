/**
 * Tests for Arena project deployment status API routes.
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
  listCrd: vi.fn(),
}));

vi.mock("@/lib/k8s/workspace-k8s-client-factory", () => ({
  getWorkspaceCoreApi: vi.fn(),
  withTokenRefresh: vi.fn((opts, fn) => fn()),
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

function createMockRequest(): NextRequest {
  const url = new URL("http://localhost:3000/api/workspaces/test-ws/arena/projects/project-1/deployment");
  return new NextRequest(url.toString(), { method: "GET" });
}

function createMockContext(projectId = "project-1") {
  return {
    params: Promise.resolve({ name: "test-ws", id: projectId }),
  };
}

describe("GET /api/workspaces/[name]/arena/projects/[id]/deployment", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
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

  it("returns deployed:false when project is not deployed", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(listCrd).mockResolvedValue([]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.deployed).toBe(false);
    expect(body.source).toBeUndefined();
  });

  it("returns deployment status when project is deployed", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    const { getWorkspaceCoreApi } = await import("@/lib/k8s/workspace-k8s-client-factory");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(listCrd).mockResolvedValue([
      {
        metadata: {
          name: "project-project-1",
          namespace: "test-ns",
          labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
          creationTimestamp: "2024-01-01T00:00:00Z",
        },
        spec: {
          type: "configmap",
          configMap: { name: "arena-project-project-1" },
          interval: "5m",
        },
        status: {
          phase: "Ready",
          artifact: { lastUpdateTime: "2024-01-01T00:00:00Z" },
        },
      },
    ]);
    vi.mocked(getWorkspaceCoreApi).mockResolvedValue({
      readNamespacedConfigMap: vi.fn().mockResolvedValue({
        metadata: {
          name: "arena-project-project-1",
          namespace: "test-ns",
          creationTimestamp: new Date("2024-01-01T00:00:00Z"),
          resourceVersion: "12345",
        },
        data: { "config.yaml": "content", "prompts__main.yaml": "content2" },
      }),
    } as unknown as Awaited<ReturnType<typeof getWorkspaceCoreApi>>);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.deployed).toBe(true);
    expect(body.source).toBeDefined();
    expect(body.source.metadata.name).toBe("project-project-1");
    expect(body.configMap).toBeDefined();
    expect(body.configMap.fileCount).toBe(2);
  });

  it("returns deployment status even when ConfigMap is missing", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    const { getWorkspaceCoreApi } = await import("@/lib/k8s/workspace-k8s-client-factory");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(listCrd).mockResolvedValue([
      {
        metadata: {
          name: "project-project-1",
          namespace: "test-ns",
          labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
          creationTimestamp: "2024-01-01T00:00:00Z",
        },
        spec: {
          type: "configmap",
          configMap: { name: "arena-project-project-1" },
          interval: "5m",
        },
        status: { phase: "Failed" },
      },
    ]);
    vi.mocked(getWorkspaceCoreApi).mockResolvedValue({
      readNamespacedConfigMap: vi.fn().mockRejectedValue({ statusCode: 404 }),
    } as unknown as Awaited<ReturnType<typeof getWorkspaceCoreApi>>);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.deployed).toBe(true);
    expect(body.source).toBeDefined();
    expect(body.configMap).toBeUndefined();
  });

  it("returns deployment status when source has no configMap spec", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(listCrd).mockResolvedValue([
      {
        metadata: {
          name: "project-project-1",
          namespace: "test-ns",
          labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
          creationTimestamp: "2024-01-01T00:00:00Z",
        },
        spec: {
          type: "git",
          git: { url: "https://github.com/example/repo" },
          interval: "5m",
        },
        status: { phase: "Ready" },
      },
    ]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.deployed).toBe(true);
    expect(body.configMap).toBeUndefined();
  });
});

/**
 * Tests for workspace-scoped prompt packs API routes (collection).
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
  listCrd: vi.fn(),
  createCrd: vi.fn(),
  createOrUpdateConfigMap: vi.fn(),
  extractK8sErrorMessage: vi.fn((error: unknown) =>
    error instanceof Error ? error.message : String(error)
  ),
  isForbiddenError: vi.fn(),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/workspace-route-helpers")>();
  return {
    ...actual,
    validateWorkspace: vi.fn(),
    getWorkspaceResource: vi.fn(),
    createAuditContext: vi.fn(() => ({
      workspace: "test-ws",
      namespace: "test-ns",
      user: { username: "testuser" },
      role: "editor",
      resourceType: "PromptPack",
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
const noPermissions = { read: false, write: false, delete: false, manageMembers: false };

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

const mockPromptPack = {
  metadata: { name: "my-pack", namespace: "test-ns" },
  spec: { source: { type: "configmap", configMapRef: { name: "my-pack-content" } } },
};

function createMockRequest(method: string, body?: unknown): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/promptpacks";
  const init: { method: string; body?: string; headers?: Record<string, string> } = { method };
  if (body) {
    init.body = JSON.stringify(body);
    init.headers = { "Content-Type": "application/json" };
  }
  return new NextRequest(url, init);
}

function createMockContext() {
  return {
    params: Promise.resolve({ name: "test-ws" }),
  };
}

describe("GET /api/workspaces/[name]/promptpacks", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns prompt packs for authenticated user with access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });
    vi.mocked(listCrd).mockResolvedValue([mockPromptPack]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toHaveLength(1);
    expect(body[0].metadata.name).toBe("my-pack");
  });

  it("returns 403 when user lacks workspace access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: false, role: null, permissions: noPermissions });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(403);
    const body = await response.json();
    expect(body.error).toBe("Forbidden");
  });

  it("returns 500 on K8s error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });
    vi.mocked(listCrd).mockRejectedValue(new Error("K8s connection failed"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(500);
    const body = await response.json();
    expect(body.error).toBe("Internal Server Error");
  });
});

describe("POST /api/workspaces/[name]/promptpacks", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("creates PromptPack without ConfigMap when content is not provided", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { createCrd, createOrUpdateConfigMap } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    });

    const createdPack = {
      metadata: { name: "my-pack", namespace: "test-ns" },
      spec: {},
    };
    vi.mocked(createCrd).mockResolvedValue(createdPack);

    const { POST } = await import("./route");
    const requestBody = {
      metadata: { name: "my-pack" },
      spec: {},
    };
    const response = await POST(createMockRequest("POST", requestBody), createMockContext());

    expect(response.status).toBe(201);
    const body = await response.json();
    expect(body.metadata.name).toBe("my-pack");
    expect(createOrUpdateConfigMap).not.toHaveBeenCalled();
  });

  it("creates ConfigMap then PromptPack when content is provided", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { createCrd, createOrUpdateConfigMap } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    });
    vi.mocked(createOrUpdateConfigMap).mockResolvedValue(undefined);

    const createdPack = {
      metadata: { name: "my-pack", namespace: "test-ns" },
      spec: { source: { type: "configmap", configMapRef: { name: "my-pack-content" } } },
    };
    vi.mocked(createCrd).mockResolvedValue(createdPack);

    const { POST } = await import("./route");
    const content = { "pack.yaml": "prompts:\n  main:\n    template: Hello" };
    const requestBody = {
      metadata: { name: "my-pack" },
      spec: {},
      content,
    };
    const response = await POST(createMockRequest("POST", requestBody), createMockContext());

    expect(response.status).toBe(201);
    expect(createOrUpdateConfigMap).toHaveBeenCalledWith(
      { workspace: "test-ws", namespace: "test-ns", role: "editor" },
      "my-pack-content",
      content,
      {
        "omnia.altairalabs.ai/workspace": "test-ws",
        "omnia.altairalabs.ai/managed-by": "promptpack",
        "omnia.altairalabs.ai/promptpack": "my-pack",
      }
    );
  });

  it("sets spec.source.configMapRef when content is provided", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { createCrd, createOrUpdateConfigMap } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    });
    vi.mocked(createOrUpdateConfigMap).mockResolvedValue(undefined);

    let capturedSpec: unknown;
    vi.mocked(createCrd).mockImplementation(async (_opts, _plural, resource) => {
      capturedSpec = (resource as any).spec;
      return resource as any;
    });

    const { POST } = await import("./route");
    const requestBody = {
      metadata: { name: "my-pack" },
      spec: { displayName: "My Pack" },
      content: { "pack.yaml": "prompts: {}" },
    };
    await POST(createMockRequest("POST", requestBody), createMockContext());

    expect(capturedSpec).toMatchObject({
      displayName: "My Pack",
      source: { type: "configmap", configMapRef: { name: "my-pack-content" } },
    });
  });

  it("returns 403 for viewer role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: false, role: "viewer", permissions: viewerPermissions });

    const { POST } = await import("./route");
    const requestBody = { metadata: { name: "my-pack" }, spec: {} };
    const response = await POST(createMockRequest("POST", requestBody), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 500 on error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { createCrd } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    });
    vi.mocked(createCrd).mockRejectedValue(new Error("Create failed"));

    const { POST } = await import("./route");
    const requestBody = { metadata: { name: "my-pack" }, spec: {} };
    const response = await POST(createMockRequest("POST", requestBody), createMockContext());

    expect(response.status).toBe(500);
  });
});

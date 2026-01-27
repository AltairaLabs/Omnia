/**
 * Tests for Arena template sources API routes.
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
  };
});

vi.mock("@/lib/k8s/crd-operations", () => ({
  listCrd: vi.fn(),
  createCrd: vi.fn(),
  extractK8sErrorMessage: vi.fn((err: unknown) => err instanceof Error ? err.message : "Unknown error"),
  isForbiddenError: vi.fn(),
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

const mockTemplateSource = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "ArenaTemplateSource",
  metadata: { name: "test-source", namespace: "test-ns" },
  spec: { type: "git", git: { url: "https://github.com/test/repo" } },
  status: { phase: "Ready", templates: [] },
};

function createMockRequest(method = "GET", body?: unknown): NextRequest {
  const url = new URL("http://localhost:3000/api/workspaces/test-ws/arena/template-sources");
  if (body) {
    return new NextRequest(url.toString(), {
      method,
      body: JSON.stringify(body),
      headers: { "Content-Type": "application/json" },
    });
  }
  return new NextRequest(url.toString(), { method });
}

function createMockContext() {
  return {
    params: Promise.resolve({ name: "test-ws" }),
  };
}

describe("GET /api/workspaces/[name]/arena/template-sources", () => {
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

  it("returns 404 when workspace is not found", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { NextResponse } = await import("next/server");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: false,
      response: NextResponse.json({ error: "Not Found", message: "Workspace not found" }, { status: 404 }),
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns list of template sources", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(listCrd).mockResolvedValue([mockTemplateSource]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toHaveLength(1);
    expect(body[0].metadata.name).toBe("test-source");
  });

  it("handles errors when listing fails", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd, isForbiddenError } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(listCrd).mockRejectedValue(new Error("K8s error"));
    vi.mocked(isForbiddenError).mockReturnValue(false);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(500);
  });

  it("returns 403 with helpful message when K8s RBAC denies access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd, isForbiddenError } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    // Simulate K8s RBAC error
    const rbacError = Object.assign(new Error("arenatemplatesources.omnia.altairalabs.ai is forbidden"), { statusCode: 403 });
    vi.mocked(listCrd).mockRejectedValue(rbacError);
    vi.mocked(isForbiddenError).mockReturnValue(true);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(403);
    const body = await response.json();
    expect(body.error).toBe("Forbidden");
    expect(body.message).toContain("Insufficient permissions");
    expect(body.message).toContain("list template sources");
  });
});

describe("POST /api/workspaces/[name]/arena/template-sources", () => {
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
    const response = await POST(
      createMockRequest("POST", { name: "new-source", spec: { type: "git" } }),
      createMockContext()
    );

    expect(response.status).toBe(403);
  });

  it("creates a new template source successfully", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { createCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(createCrd).mockResolvedValue(mockTemplateSource);

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest("POST", {
        name: "test-source",
        spec: { type: "git", git: { url: "https://github.com/test/repo" } },
      }),
      createMockContext()
    );

    expect(response.status).toBe(201);
    const body = await response.json();
    expect(body.metadata.name).toBe("test-source");
  });

  it("handles errors when creation fails", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { createCrd, isForbiddenError } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(createCrd).mockRejectedValue(new Error("Creation failed"));
    vi.mocked(isForbiddenError).mockReturnValue(false);

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest("POST", { name: "test-source", spec: { type: "git" } }),
      createMockContext()
    );

    expect(response.status).toBe(500);
  });

  it("returns 403 with helpful message when K8s RBAC denies create access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { createCrd, isForbiddenError } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: {},
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    // Simulate K8s RBAC error
    const rbacError = Object.assign(new Error("arenatemplatesources.omnia.altairalabs.ai is forbidden"), { statusCode: 403 });
    vi.mocked(createCrd).mockRejectedValue(rbacError);
    vi.mocked(isForbiddenError).mockReturnValue(true);

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest("POST", { name: "test-source", spec: { type: "git" } }),
      createMockContext()
    );

    expect(response.status).toBe(403);
    const body = await response.json();
    expect(body.error).toBe("Forbidden");
    expect(body.message).toContain("Insufficient permissions");
    expect(body.message).toContain("create template source");
  });
});

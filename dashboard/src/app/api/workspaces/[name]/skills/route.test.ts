/**
 * Tests for workspace-scoped skill sources API routes. Issue #829.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/k8s/crd-operations", () => ({
  listCrd: vi.fn(),
  createCrd: vi.fn(),
  extractK8sErrorMessage: vi.fn((error: unknown) =>
    error instanceof Error ? error.message : String(error)
  ),
  isForbiddenError: vi.fn(),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/lib/k8s/workspace-route-helpers")>();
  return {
    ...actual,
    validateWorkspace: vi.fn(),
    getWorkspaceResource: vi.fn(),
    createAuditContext: vi.fn(() => ({
      workspace: "test-ws",
      namespace: "test-ns",
      user: { username: "testuser" },
      role: "editor",
      resourceType: "SkillSource",
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

const viewerPermissions = {
  read: true,
  write: false,
  delete: false,
  manageMembers: false,
};
const editorPermissions = {
  read: true,
  write: true,
  delete: true,
  manageMembers: false,
};
const noPermissions = {
  read: false,
  write: false,
  delete: false,
  manageMembers: false,
};

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

const mockSource = {
  metadata: { name: "skills-git", namespace: "test-ns" },
  spec: {
    type: "git",
    git: { url: "https://example.com/skills.git" },
    interval: "1h",
  },
  status: { phase: "Ready", skillCount: 3 },
};

function createMockRequest(method: string, body?: unknown): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/skills";
  const init: { method: string; body?: string; headers?: Record<string, string> } = {
    method,
  };
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

describe("GET /api/workspaces/[name]/skills", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns skill sources for authenticated user with access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import(
      "@/lib/k8s/workspace-route-helpers"
    );

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
       
      workspace: mockWorkspace as any,
      clientOptions: {
        workspace: "test-ws",
        namespace: "test-ns",
        role: "viewer",
      },
    });
    vi.mocked(listCrd).mockResolvedValue([mockSource]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toHaveLength(1);
    expect(body[0].metadata.name).toBe("skills-git");
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
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 500 on K8s error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import(
      "@/lib/k8s/workspace-route-helpers"
    );

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
       
      workspace: mockWorkspace as any,
      clientOptions: {
        workspace: "test-ws",
        namespace: "test-ns",
        role: "viewer",
      },
    });
    vi.mocked(listCrd).mockRejectedValue(new Error("K8s connection failed"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(500);
  });
});

describe("POST /api/workspaces/[name]/skills", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("creates skill source for user with editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { createCrd } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import(
      "@/lib/k8s/workspace-route-helpers"
    );

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "editor",
      permissions: editorPermissions,
    });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
       
      workspace: mockWorkspace as any,
      clientOptions: {
        workspace: "test-ws",
        namespace: "test-ns",
        role: "editor",
      },
    });
    vi.mocked(createCrd).mockResolvedValue(mockSource);

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest("POST", {
        metadata: { name: "skills-git" },
        spec: {
          type: "git",
          git: { url: "https://example.com/skills.git" },
          interval: "1h",
        },
      }),
      createMockContext()
    );

    expect(response.status).toBe(201);
  });

  it("returns 403 when user lacks editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: false,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest("POST", {
        metadata: { name: "skills-git" },
        spec: { type: "git", interval: "1h" },
      }),
      createMockContext()
    );

    expect(response.status).toBe(403);
  });
});

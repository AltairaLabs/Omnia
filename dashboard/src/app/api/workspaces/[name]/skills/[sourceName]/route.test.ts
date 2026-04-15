/**
 * Tests for individual workspace-scoped SkillSource API routes. Issue #829.
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
  getCrd: vi.fn(),
  updateCrd: vi.fn(),
  deleteCrd: vi.fn(),
  extractK8sErrorMessage: vi.fn((error: unknown) =>
    error instanceof Error ? error.message : String(error)
  ),
  isForbiddenError: vi.fn(() => false),
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
};

function createMockRequest(method: string, body?: unknown): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/skills/skills-git";
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
    params: Promise.resolve({ name: "test-ws", sourceName: "skills-git" }),
  };
}

describe("GET /api/workspaces/[name]/skills/[sourceName]", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns the skill source for users with read access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import(
      "@/lib/k8s/workspace-route-helpers"
    );

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: mockSource,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      workspace: mockWorkspace as any,
      clientOptions: {
        workspace: "test-ws",
        namespace: "test-ns",
        role: "viewer",
      },
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.metadata.name).toBe("skills-git");
  });

  it("returns 403 when the user lacks workspace access", async () => {
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

  it("returns 404 when the skill source does not exist", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource, notFoundResponse } = await import(
      "@/lib/k8s/workspace-route-helpers"
    );

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: false,
      response: notFoundResponse("Skill source not found: skills-git"),
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(404);
  });
});

describe("DELETE /api/workspaces/[name]/skills/[sourceName]", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("deletes the skill source for users with editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { deleteCrd } = await import("@/lib/k8s/crd-operations");
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
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      workspace: mockWorkspace as any,
      clientOptions: {
        workspace: "test-ws",
        namespace: "test-ns",
        role: "editor",
      },
    });
    vi.mocked(deleteCrd).mockResolvedValue(undefined);

    const { DELETE } = await import("./route");
    const response = await DELETE(
      createMockRequest("DELETE"),
      createMockContext()
    );

    expect(response.status).toBe(204);
  });

  it("returns 403 when the user lacks editor access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: false,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const { DELETE } = await import("./route");
    const response = await DELETE(
      createMockRequest("DELETE"),
      createMockContext()
    );

    expect(response.status).toBe(403);
  });
});

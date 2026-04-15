/**
 * Tests for SkillSource content API route.
 * GET /api/workspaces/:name/skills/:sourceName/content
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({ getUser: vi.fn() }));
vi.mock("@/lib/auth/workspace-authz", () => ({ checkWorkspaceAccess: vi.fn() }));

vi.mock("@/lib/k8s/crd-operations", () => ({
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
      role: "viewer",
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

vi.mock("node:fs", () => ({
  existsSync: vi.fn(),
  readdirSync: vi.fn(),
  readFileSync: vi.fn(),
  statSync: vi.fn(),
}));

const mockUser = {
  id: "u",
  provider: "oauth" as const,
  username: "testuser",
  email: "t@e.co",
  groups: ["users"],
  role: "viewer" as const,
};
const viewerPermissions = { read: true, write: false, delete: false, manageMembers: false };
const noPermissions = { read: false, write: false, delete: false, manageMembers: false };

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

const readySource = {
  metadata: { name: "skills-git", namespace: "test-ns" },
  spec: { type: "git" },
  status: { phase: "Ready" },
};
const pendingSource = {
  metadata: { name: "skills-git", namespace: "test-ns" },
  spec: { type: "git" },
  status: { phase: "Pending" },
};

function createMockRequest(): NextRequest {
  return new NextRequest(
    "http://localhost:3000/api/workspaces/test-ws/skills/skills-git/content",
    { method: "GET" }
  );
}
function createMockContext() {
  return { params: Promise.resolve({ name: "test-ws", sourceName: "skills-git" }) };
}

describe("GET /api/workspaces/[name]/skills/[sourceName]/content", () => {
  beforeEach(() => vi.resetModules());
  afterEach(() => vi.resetAllMocks());

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
    const response = await GET(createMockRequest(), createMockContext());
    expect(response.status).toBe(403);
  });

  it("returns 404 when source phase is not Ready and basePath is missing", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs");
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: pendingSource,

      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });
    vi.mocked(fs.existsSync).mockReturnValue(false);
    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());
    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.message).toContain("not ready");
  });

  it("returns content tree when HEAD version exists", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs");
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: readySource,

      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });
    vi.mocked(fs.existsSync).mockReturnValue(true);
    vi.mocked(fs.readFileSync).mockReturnValue("v1");
    vi.mocked(fs.readdirSync).mockImplementation((dir: unknown, _opts: unknown) => {
      const dirStr = String(dir);
      if (dirStr.endsWith("v1")) {
        return [
          { name: "SKILL.md", isDirectory: () => false, isFile: () => true },
          { name: "subdir", isDirectory: () => true, isFile: () => false },
        ] as never;
      }
      if (dirStr.endsWith("v1/subdir")) {
        return [
          { name: "nested.txt", isDirectory: () => false, isFile: () => true },
        ] as never;
      }
      return [] as never;
    });
    vi.mocked(fs.statSync).mockReturnValue({ size: 256 } as never);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.sourceName).toBe("skills-git");
    expect(body.fileCount).toBe(2);
    expect(body.directoryCount).toBe(1);
    expect(body.tree[0].name).toBe("subdir");
  });

  it("returns 500 on K8s error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    vi.mocked(getWorkspaceResource).mockRejectedValue(new Error("boom"));
    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());
    expect(response.status).toBe(500);
  });
});

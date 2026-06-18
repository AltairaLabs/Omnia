/**
 * Tests for SkillSource content API route.
 * GET /api/workspaces/:name/skills/:sourceName/content
 *
 * The route now calls the operator content API via content-api-service /
 * content-tree; these mock those services (mock-to-contract: shapes match the
 * Go content.Listing / content.FileContent json tags). The SkillSource CRD
 * lookup (getWorkspaceResource) is still mocked for targetPath + phase checks.
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

vi.mock("@/lib/data/content-api-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/data/content-api-service")>();
  return { ...actual, getContent: vi.fn() };
});

vi.mock("@/lib/data/content-tree", () => ({ listContentTree: vi.fn() }));

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

async function grantAccess() {
  const { getUser } = await import("@/lib/auth");
  const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
  vi.mocked(getUser).mockResolvedValue(mockUser);
  vi.mocked(checkWorkspaceAccess).mockResolvedValue({
    granted: true,
    role: "viewer",
    permissions: viewerPermissions,
  });
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

  it("returns 404 when source phase is not Ready and no content exists", async () => {
    await grantAccess();
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: pendingSource,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });
    // HEAD missing, base listing missing -> no content resolved.
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());
    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.message).toContain("not ready");
  });

  it("returns content tree when HEAD version exists", async () => {
    await grantAccess();
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const svc = await import("@/lib/data/content-api-service");
    const { listContentTree } = await import("@/lib/data/content-tree");
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: readySource,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath.endsWith("/.arena/HEAD")) {
        return { path: relpath, content: "v1\n", encoding: "utf-8", size: 3, modifiedAt: "t" };
      }
      // The resolved version directory listing has visible entries.
      return {
        path: relpath,
        entries: [
          { name: "SKILL.md", type: "file", size: 256, modifiedAt: "t" },
        ],
      };
    });
    // content root is skills/skills-git/.arena/versions/v1
    vi.mocked(listContentTree).mockResolvedValue([
      {
        name: "subdir",
        path: "skills/skills-git/.arena/versions/v1/subdir",
        isDirectory: true,
        modifiedAt: "t",
        children: [
          {
            name: "nested.txt",
            path: "skills/skills-git/.arena/versions/v1/subdir/nested.txt",
            isDirectory: false,
            size: 10,
            modifiedAt: "t",
          },
        ],
      },
      {
        name: "SKILL.md",
        path: "skills/skills-git/.arena/versions/v1/SKILL.md",
        isDirectory: false,
        size: 256,
        modifiedAt: "t",
      },
    ]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.sourceName).toBe("skills-git");
    expect(body.fileCount).toBe(2);
    expect(body.directoryCount).toBe(1);
    // Directories sort first; paths are relative to the content root.
    expect(body.tree[0].name).toBe("subdir");
    expect(body.tree[0].path).toBe("subdir");
    expect(body.tree[0].children[0].path).toBe("subdir/nested.txt");
    expect(body.tree[1].name).toBe("SKILL.md");
  });

  it("falls back to the base path when there is no HEAD", async () => {
    await grantAccess();
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const svc = await import("@/lib/data/content-api-service");
    const { listContentTree } = await import("@/lib/data/content-tree");
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: readySource,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath.endsWith("/.arena/HEAD")) {
        throw new svc.ContentApiError("not found", 404);
      }
      // The base listing has a visible entry.
      return {
        path: relpath,
        entries: [{ name: "SKILL.md", type: "file", size: 256, modifiedAt: "t" }],
      };
    });
    vi.mocked(listContentTree).mockResolvedValue([
      {
        name: "SKILL.md",
        path: "skills/skills-git/SKILL.md",
        isDirectory: false,
        size: 256,
        modifiedAt: "t",
      },
    ]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.fileCount).toBe(1);
    expect(body.tree[0].path).toBe("SKILL.md");
    expect(vi.mocked(listContentTree)).toHaveBeenCalledWith(
      "test-ws",
      mockUser,
      "skills/skills-git",
      { skipHidden: true }
    );
  });

  it("returns 500 on K8s error", async () => {
    await grantAccess();
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspaceResource).mockRejectedValue(new Error("boom"));
    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());
    expect(response.status).toBe(500);
  });
});

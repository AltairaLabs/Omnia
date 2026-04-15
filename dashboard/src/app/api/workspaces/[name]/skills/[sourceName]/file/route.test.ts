/**
 * Tests for SkillSource file API route.
 * GET /api/workspaces/:name/skills/:sourceName/file?path=...
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

function makeReq(query?: string): NextRequest {
  const url = `http://localhost:3000/api/workspaces/test-ws/skills/skills-git/file${
    query ?? ""
  }`;
  return new NextRequest(url, { method: "GET" });
}
function ctx() {
  return { params: Promise.resolve({ name: "test-ws", sourceName: "skills-git" }) };
}

function setAccess() {
  return Promise.all([import("@/lib/auth"), import("@/lib/auth/workspace-authz")]).then(
    ([{ getUser }, { checkWorkspaceAccess }]) => {
      vi.mocked(getUser).mockResolvedValue(mockUser);
      vi.mocked(checkWorkspaceAccess).mockResolvedValue({
        granted: true,
        role: "viewer",
        permissions: viewerPermissions,
      });
    }
  );
}

describe("GET /api/workspaces/[name]/skills/[sourceName]/file", () => {
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
    const response = await GET(makeReq("?path=SKILL.md"), ctx());
    expect(response.status).toBe(403);
  });

  it("returns 400 when path query is missing", async () => {
    await setAccess();
    const { GET } = await import("./route");
    const response = await GET(makeReq(), ctx());
    expect(response.status).toBe(400);
  });

  it("rejects path-traversal attempts", async () => {
    await setAccess();
    const { GET } = await import("./route");
    const response = await GET(makeReq("?path=../etc/passwd"), ctx());
    expect(response.status).toBe(400);
  });

  it("returns file contents when path is valid", async () => {
    await setAccess();
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs");
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: readySource,

      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });
    vi.mocked(fs.existsSync).mockReturnValue(true);
    vi.mocked(fs.readFileSync).mockImplementation((p: unknown) => {
      const s = String(p);
      if (s.endsWith("HEAD")) return "v1";
      return "# Hello";
    });
    vi.mocked(fs.statSync).mockReturnValue({
      size: 7,
      isDirectory: () => false,
    } as never);

    const { GET } = await import("./route");
    const response = await GET(makeReq("?path=SKILL.md"), ctx());
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.content).toBe("# Hello");
    expect(body.size).toBe(7);
  });

  it("returns 404 when the file is missing", async () => {
    await setAccess();
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs");
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: readySource,

      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });
    vi.mocked(fs.existsSync).mockImplementation((p: unknown) => {
      const s = String(p);
      // basePath, HEAD and version dir exist; final fullPath does not.
      return !s.endsWith("missing.md");
    });
    vi.mocked(fs.readFileSync).mockReturnValue("v1");

    const { GET } = await import("./route");
    const response = await GET(makeReq("?path=missing.md"), ctx());
    expect(response.status).toBe(404);
  });

  it("returns 413 when file exceeds the size limit", async () => {
    await setAccess();
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs");
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: readySource,

      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });
    vi.mocked(fs.existsSync).mockReturnValue(true);
    vi.mocked(fs.readFileSync).mockReturnValue("v1");
    vi.mocked(fs.statSync).mockReturnValue({
      size: 5 * 1024 * 1024,
      isDirectory: () => false,
    } as never);

    const { GET } = await import("./route");
    const response = await GET(makeReq("?path=big.md"), ctx());
    expect(response.status).toBe(413);
  });

  it("returns 500 on K8s error", async () => {
    await setAccess();
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspaceResource).mockRejectedValue(new Error("boom"));
    const { GET } = await import("./route");
    const response = await GET(makeReq("?path=SKILL.md"), ctx());
    expect(response.status).toBe(500);
  });
});

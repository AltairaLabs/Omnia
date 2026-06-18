/**
 * Tests for SkillSource file API route.
 * GET /api/workspaces/:name/skills/:sourceName/file?path=...
 *
 * The route now calls the operator content API via content-api-service; these
 * mock that service (mock-to-contract: shapes match the Go content.Listing /
 * content.FileContent json tags). Path-confinement and max-size are operator-
 * side, surfaced here as pass-through statuses. The SkillSource CRD lookup
 * (getWorkspaceResource) is still mocked for targetPath resolution.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

import type { User } from "@/lib/auth/types";

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

async function mockReadySource() {
  const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
  vi.mocked(getWorkspaceResource).mockResolvedValue({
    ok: true,
    resource: readySource,
    workspace: mockWorkspace as any,
    clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
  });
}

/**
 * Make getContent resolve the HEAD pointer + version-dir listing, then call the
 * given fileImpl for the actual file fetch (path under the resolved version dir).
 */
function contentImpl(
  svc: typeof import("@/lib/data/content-api-service"),
  fileImpl: (relpath: string) => Awaited<ReturnType<typeof svc.getContent>>
) {
  return async (
    _ws: string,
    _user: User,
    relpath = ""
  ): Promise<Awaited<ReturnType<typeof svc.getContent>>> => {
    if (relpath.endsWith("/.arena/HEAD")) {
      return { path: relpath, content: "v1", encoding: "utf-8", size: 2, modifiedAt: "t" };
    }
    if (relpath.endsWith("/.arena/versions/v1")) {
      return {
        path: relpath,
        entries: [{ name: "SKILL.md", type: "file", size: 7, modifiedAt: "t" }],
      };
    }
    return fileImpl(relpath);
  };
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

  it("returns file contents when path is valid", async () => {
    await setAccess();
    await mockReadySource();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(
      contentImpl(svc, (relpath) => ({
        path: relpath,
        content: "# Hello",
        encoding: "utf-8",
        size: 7,
        modifiedAt: "t",
      }))
    );

    const { GET } = await import("./route");
    const response = await GET(makeReq("?path=SKILL.md"), ctx());
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.path).toBe("SKILL.md");
    expect(body.content).toBe("# Hello");
    expect(body.size).toBe(7);
  });

  it("passes through 404 when the file is missing", async () => {
    await setAccess();
    await mockReadySource();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(
      contentImpl(svc, () => {
        throw new svc.ContentApiError("not found", 404);
      })
    );

    const { GET } = await import("./route");
    const response = await GET(makeReq("?path=missing.md"), ctx());
    expect(response.status).toBe(404);
  });

  it("passes through 400 for a path the operator rejects (traversal)", async () => {
    await setAccess();
    await mockReadySource();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(
      contentImpl(svc, () => {
        throw new svc.ContentApiError("invalid path", 400);
      })
    );

    const { GET } = await import("./route");
    const response = await GET(makeReq("?path=../etc/passwd"), ctx());
    expect(response.status).toBe(400);
  });

  it("returns 400 when the requested path is a directory", async () => {
    await setAccess();
    await mockReadySource();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(
      contentImpl(svc, (relpath) => ({ path: relpath, entries: [] }))
    );

    const { GET } = await import("./route");
    const response = await GET(makeReq("?path=subdir"), ctx());
    expect(response.status).toBe(400);
    expect((await response.json()).error).toBe("Path is a directory");
  });

  it("passes through 413 when the operator rejects the file as too large", async () => {
    await setAccess();
    await mockReadySource();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(
      contentImpl(svc, () => {
        throw new svc.ContentApiError("file too large", 413);
      })
    );

    const { GET } = await import("./route");
    const response = await GET(makeReq("?path=big.md"), ctx());
    expect(response.status).toBe(413);
  });

  it("returns 404 when the source has no resolvable content", async () => {
    await setAccess();
    await mockReadySource();
    const svc = await import("@/lib/data/content-api-service");
    // HEAD missing and base listing missing -> no content path.
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { GET } = await import("./route");
    const response = await GET(makeReq("?path=SKILL.md"), ctx());
    expect(response.status).toBe(404);
    expect((await response.json()).error).toBe("Skill source content not available");
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

/**
 * Tests for Arena source content API route.
 *
 * GET /api/workspaces/:name/arena/sources/:sourceName/content - Get source file tree
 *
 * The route now calls the operator content API (content-api-service /
 * content-tree); these mock those modules instead of node:fs. Mock shapes match
 * the Go content.Listing / content.FileContent json tags (mock-to-contract).
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
  extractK8sErrorMessage: vi.fn((error: unknown) =>
    error instanceof Error ? error.message : String(error)
  ),
  isForbiddenError: vi.fn(() => false),
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
      role: "viewer",
      resourceType: "ArenaSource",
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

vi.mock("@/lib/data/content-tree", () => ({
  listContentTree: vi.fn(),
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

const mockSourceReady = {
  metadata: { name: "test-source", namespace: "test-ns" },
  spec: { type: "git" },
  status: { phase: "Ready" },
};

const mockSourcePending = {
  metadata: { name: "test-source", namespace: "test-ns" },
  spec: { type: "git" },
  status: { phase: "Pending" },
};

const T = "2025-01-01T00:00:00Z";

function createMockRequest(): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/arena/sources/test-source/content";
  return new NextRequest(url, { method: "GET" });
}

function createMockContext() {
  return {
    params: Promise.resolve({ name: "test-ws", sourceName: "test-source" }),
  };
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

async function mockResource(resource: unknown) {
  const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
  vi.mocked(getWorkspaceResource).mockResolvedValue({
    ok: true,
    resource,
    workspace: mockWorkspace,
    clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
  } as any);
}

describe("GET /api/workspaces/[name]/arena/sources/[sourceName]/content", () => {
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

  it("returns 404 when source is not found", async () => {
    await grantAccess();
    const { getWorkspaceResource, notFoundResponse } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: false,
      response: notFoundResponse("Arena source not found"),
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns 404 when source directory does not exist and source is not ready", async () => {
    await grantAccess();
    await mockResource(mockSourcePending);
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
    expect((await response.json()).message).toContain("not ready");
  });

  it("returns 404 when source directory does not exist and source is ready", async () => {
    await grantAccess();
    await mockResource(mockSourceReady);
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
    expect((await response.json()).message).toContain("re-synced");
  });

  it("returns 404 when no content is found (no HEAD and no direct content)", async () => {
    await grantAccess();
    await mockResource(mockSourceReady);
    const svc = await import("@/lib/data/content-api-service");
    // base path exists but holds only the hidden .arena dir; no HEAD file.
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath.endsWith("/.arena/HEAD")) {
        throw new svc.ContentApiError("not found", 404);
      }
      return { path: relpath, entries: [{ name: ".arena", type: "directory", size: 0, modifiedAt: T }] };
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
    expect((await response.json()).message).toContain("No content found");
  });

  it("returns content tree when HEAD version exists", async () => {
    await grantAccess();
    await mockResource(mockSourceReady);
    const svc = await import("@/lib/data/content-api-service");
    const tree = await import("@/lib/data/content-tree");

    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath.endsWith("/.arena/HEAD")) {
        return { path: relpath, content: "abc123\n", encoding: "utf-8", size: 7, modifiedAt: T };
      }
      // base path + resolved version dir exist
      return { path: relpath, entries: [{ name: "x", type: "file", size: 1, modifiedAt: T }] };
    });

    const versionRoot = "arena/test-source/.arena/versions/abc123";
    vi.mocked(tree.listContentTree).mockResolvedValue([
      { name: "config.arena.yaml", path: `${versionRoot}/config.arena.yaml`, isDirectory: false, size: 1024, modifiedAt: T },
      {
        name: "scenarios",
        path: `${versionRoot}/scenarios`,
        isDirectory: true,
        modifiedAt: T,
        children: [
          { name: "test.yaml", path: `${versionRoot}/scenarios/test.yaml`, isDirectory: false, size: 10, modifiedAt: T },
        ],
      },
    ]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.sourceName).toBe("test-source");
    expect(body.tree.length).toBe(2);
    expect(body.fileCount).toBe(2);
    expect(body.directoryCount).toBe(1);

    // Directories first, paths made relative to the content root.
    expect(body.tree[0].name).toBe("scenarios");
    expect(body.tree[0].isDirectory).toBe(true);
    expect(body.tree[0].path).toBe("scenarios");
    expect(body.tree[0].children.length).toBe(1);
    expect(body.tree[0].children[0].path).toBe("scenarios/test.yaml");

    expect(body.tree[1].name).toBe("config.arena.yaml");
    expect(body.tree[1].isDirectory).toBe(false);
    expect(body.tree[1].size).toBe(1024);

    // skipHidden requested so the .arena dir is excluded.
    expect(vi.mocked(tree.listContentTree)).toHaveBeenCalledWith("test-ws", mockUser, versionRoot, {
      skipHidden: true,
    });
  });

  it("falls back to base path when HEAD points to a non-existent version", async () => {
    await grantAccess();
    await mockResource(mockSourceReady);
    const svc = await import("@/lib/data/content-api-service");
    const tree = await import("@/lib/data/content-tree");

    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath.endsWith("/.arena/HEAD")) {
        return { path: relpath, content: "badversion\n", encoding: "utf-8", size: 11, modifiedAt: T };
      }
      if (relpath.endsWith("/versions/badversion")) {
        throw new svc.ContentApiError("not found", 404);
      }
      // base path exists with legacy content
      return { path: relpath, entries: [{ name: "legacy-file.yaml", type: "file", size: 512, modifiedAt: T }] };
    });

    vi.mocked(tree.listContentTree).mockResolvedValue([
      { name: "legacy-file.yaml", path: "arena/test-source/legacy-file.yaml", isDirectory: false, size: 512, modifiedAt: T },
    ]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.tree.length).toBe(1);
    expect(body.tree[0].name).toBe("legacy-file.yaml");
    expect(body.tree[0].path).toBe("legacy-file.yaml");
    // Tree built from the base path, not a version dir.
    expect(vi.mocked(tree.listContentTree)).toHaveBeenCalledWith("test-ws", mockUser, "arena/test-source", {
      skipHidden: true,
    });
  });

  it("returns 500 when the content API raises a non-404 error", async () => {
    await grantAccess();
    await mockResource(mockSourceReady);
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("boom", 500));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(500);
  });

  it("handles K8s errors gracefully", async () => {
    await grantAccess();
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspaceResource).mockRejectedValue(new Error("K8s connection failed"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(500);
  });
});

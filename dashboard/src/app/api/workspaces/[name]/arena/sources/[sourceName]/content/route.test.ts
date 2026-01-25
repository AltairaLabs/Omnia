/**
 * Tests for Arena source content API route.
 *
 * GET /api/workspaces/:name/arena/sources/:sourceName/content - Get source file tree
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

// Mock fs for filesystem content reading
vi.mock("node:fs", () => ({
  existsSync: vi.fn(),
  readdirSync: vi.fn(),
  readFileSync: vi.fn(),
  statSync: vi.fn(),
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

function createMockRequest(): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/arena/sources/test-source/content";
  return new NextRequest(url, { method: "GET" });
}

function createMockContext() {
  return {
    params: Promise.resolve({ name: "test-ws", sourceName: "test-source" }),
  };
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
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource, notFoundResponse } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: false,
      response: notFoundResponse("Arena source not found"),
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns 404 when source directory does not exist and source is not ready", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: mockSourcePending,
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

  it("returns 404 when source directory does not exist and source is ready", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: mockSourceReady,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });

    vi.mocked(fs.existsSync).mockReturnValue(false);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.message).toContain("re-synced");
  });

  it("returns 404 when no content is found (no HEAD and no direct content)", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: mockSourceReady,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });

    // Base path exists, but no HEAD file and no content files
    vi.mocked(fs.existsSync).mockImplementation((p: any) => {
      const pathStr = p.toString();
      if (pathStr.includes("HEAD")) return false;
      if (pathStr.includes("versions")) return false;
      // Base path exists
      return pathStr.includes("arena/test-source");
    });
    vi.mocked(fs.readdirSync).mockReturnValue([".arena"] as any);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.message).toContain("No content found");
  });

  it("returns content tree when HEAD version exists", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: mockSourceReady,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });

    vi.mocked(fs.existsSync).mockReturnValue(true);
    vi.mocked(fs.readFileSync).mockImplementation((p: any) => {
      if (p.toString().includes("HEAD")) return "abc123";
      return "";
    });
    vi.mocked(fs.readdirSync).mockImplementation((dir: any, _opts: any) => {
      const dirStr = dir.toString();
      // Root of content (abc123 version)
      if (dirStr.endsWith("abc123")) {
        return [
          { name: "config.arena.yaml", isDirectory: () => false, isFile: () => true },
          { name: "scenarios", isDirectory: () => true, isFile: () => false },
        ] as any;
      }
      // Scenarios subdirectory (exact match)
      if (dirStr.endsWith("abc123/scenarios")) {
        return [
          { name: "test.yaml", isDirectory: () => false, isFile: () => true },
        ] as any;
      }
      return [];
    });
    vi.mocked(fs.statSync).mockReturnValue({ size: 1024 } as any);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.sourceName).toBe("test-source");
    expect(body.tree.length).toBe(2);
    expect(body.fileCount).toBe(2);
    expect(body.directoryCount).toBe(1);

    // Directories should come first
    expect(body.tree[0].name).toBe("scenarios");
    expect(body.tree[0].isDirectory).toBe(true);
    expect(body.tree[0].children.length).toBe(1);

    expect(body.tree[1].name).toBe("config.arena.yaml");
    expect(body.tree[1].isDirectory).toBe(false);
    expect(body.tree[1].size).toBe(1024);
  });

  it("falls back to base path when HEAD points to non-existent version", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: mockSourceReady,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });

    vi.mocked(fs.existsSync).mockImplementation((p: any) => {
      const pathStr = p.toString();
      // HEAD file exists
      if (pathStr.endsWith("HEAD")) return true;
      // Version directory from HEAD doesn't exist
      if (pathStr.includes("versions/badversion")) return false;
      // Base source path exists
      if (pathStr.endsWith("arena/test-source")) return true;
      return false;
    });
    vi.mocked(fs.readFileSync).mockImplementation((p: any) => {
      if (p.toString().includes("HEAD")) return "badversion";
      return "";
    });
    vi.mocked(fs.readdirSync).mockImplementation((dir: any, opts: any) => {
      const dirStr = dir.toString();
      // Base source path has legacy content (not just .arena folder)
      if (dirStr.endsWith("arena/test-source")) {
        // Without withFileTypes (used in resolveContentPath check)
        if (!opts?.withFileTypes) {
          return ["legacy-file.yaml", ".arena"] as any;
        }
        // With withFileTypes (used in buildContentTree)
        return [
          { name: "legacy-file.yaml", isDirectory: () => false, isFile: () => true },
        ] as any;
      }
      return [];
    });
    vi.mocked(fs.statSync).mockReturnValue({ size: 512 } as any);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.tree.length).toBe(1);
    expect(body.tree[0].name).toBe("legacy-file.yaml");
  });

  it("skips hidden files and directories", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: mockSourceReady,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });

    vi.mocked(fs.existsSync).mockReturnValue(true);
    vi.mocked(fs.readFileSync).mockReturnValue("abc123");
    vi.mocked(fs.readdirSync).mockImplementation((dir: any, _opts: any) => {
      if (dir.toString().includes("abc123")) {
        return [
          { name: ".hidden", isDirectory: () => true, isFile: () => false },
          { name: ".gitignore", isDirectory: () => false, isFile: () => true },
          { name: "visible.yaml", isDirectory: () => false, isFile: () => true },
        ] as any;
      }
      return [];
    });
    vi.mocked(fs.statSync).mockReturnValue({ size: 100 } as any);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.tree.length).toBe(1);
    expect(body.tree[0].name).toBe("visible.yaml");
  });

  it("handles K8s errors gracefully", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource).mockRejectedValue(new Error("K8s connection failed"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(500);
  });

  it("handles filesystem read errors gracefully", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: mockSourceReady,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });

    vi.mocked(fs.existsSync).mockReturnValue(true);
    vi.mocked(fs.readFileSync).mockReturnValue("abc123");
    vi.mocked(fs.readdirSync).mockImplementation((dir: any, _opts: any) => {
      if (dir.toString().includes("abc123")) {
        throw new Error("Permission denied");
      }
      return [];
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    // Should return 200 with empty tree (error is logged but handled)
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.tree).toEqual([]);
  });
});

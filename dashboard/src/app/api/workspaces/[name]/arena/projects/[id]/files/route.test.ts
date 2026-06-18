/**
 * Tests for Arena project files listing API routes.
 *
 * The routes now call the operator content API via content-api-service and
 * content-tree instead of reading the NFS mount (mock-to-contract: shapes match
 * the Go content.Listing / content.WriteResult json tags).
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

vi.mock("@/lib/data/content-api-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/data/content-api-service")>();
  return {
    ...actual,
    getContent: vi.fn(),
    writeContentFile: vi.fn(),
    makeContentDir: vi.fn(),
  };
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
  role: "editor" as const,
};

const editorPermissions = { read: true, write: true, delete: true, manageMembers: false };
const noPermissions = { read: false, write: false, delete: false, manageMembers: false };

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

function createMockRequest(method = "GET", body?: unknown): NextRequest {
  const url = new URL("http://localhost:3000/api/workspaces/test-ws/arena/projects/project-1/files");
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
    params: Promise.resolve({ name: "test-ws", id: "project-1" }),
  };
}

async function grantWorkspace(role: "viewer" | "editor") {
  const { getUser } = await import("@/lib/auth");
  const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
  const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
  vi.mocked(getUser).mockResolvedValue(mockUser);
  vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role, permissions: editorPermissions });
  vi.mocked(validateWorkspace).mockResolvedValue({
    ok: true,
    workspace: mockWorkspace,
  } as Awaited<ReturnType<typeof validateWorkspace>>);
}

describe("GET /api/workspaces/[name]/arena/projects/[id]/files", () => {
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

  it("returns 404 when project does not exist", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns file tree", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    const { listContentTree } = await import("@/lib/data/content-tree");
    vi.mocked(svc.getContent).mockResolvedValue({ path: "arena/projects/project-1", entries: [] });
    vi.mocked(listContentTree).mockResolvedValue([
      { name: "config.arena.yaml", path: "config.arena.yaml", isDirectory: false, size: 80, modifiedAt: "2024-01-01T00:00:00Z" },
      { name: "prompts", path: "prompts", isDirectory: true, modifiedAt: "2024-01-01T00:00:00Z", children: [] },
    ]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.tree).toBeDefined();
    expect(Array.isArray(body.tree)).toBe(true);
  });

  it("returns empty tree for empty project", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    const { listContentTree } = await import("@/lib/data/content-tree");
    vi.mocked(svc.getContent).mockResolvedValue({ path: "arena/projects/project-1", entries: [] });
    vi.mocked(listContentTree).mockResolvedValue([]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.tree).toEqual([]);
  });

  it("sorts the tree directories-first, then alphabetically", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    const { listContentTree } = await import("@/lib/data/content-tree");
    vi.mocked(svc.getContent).mockResolvedValue({ path: "arena/projects/project-1", entries: [] });
    vi.mocked(listContentTree).mockResolvedValue([
      { name: "zfile.yaml", path: "zfile.yaml", isDirectory: false, size: 10, modifiedAt: "t" },
      { name: "prompts", path: "prompts", isDirectory: true, modifiedAt: "t", children: [] },
      { name: "config.arena.yaml", path: "config.arena.yaml", isDirectory: false, size: 10, modifiedAt: "t" },
      { name: "adir", path: "adir", isDirectory: true, modifiedAt: "t", children: [] },
    ]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.tree[0].name).toBe("adir");
    expect(body.tree[1].name).toBe("prompts");
    expect(body.tree[2].name).toBe("config.arena.yaml");
    expect(body.tree[3].name).toBe("zfile.yaml");
  });
});

describe("POST /api/workspaces/[name]/arena/projects/[id]/files", () => {
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
    const response = await POST(createMockRequest("POST", { name: "new-file.yaml", isDirectory: false }), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 400 for invalid filename", async () => {
    await grantWorkspace("editor");

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "../malicious", isDirectory: false }), createMockContext());

    expect(response.status).toBe(400);
  });

  it("returns 404 when project does not exist", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "new-file.yaml", isDirectory: false }), createMockContext());

    expect(response.status).toBe(404);
  });

  it("creates a new file at root", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === "arena/projects/project-1") {
        return { path: relpath, entries: [] };
      }
      throw new svc.ContentApiError("not found", 404); // target does not exist
    });
    vi.mocked(svc.writeContentFile).mockResolvedValue({ path: "x", size: 0, modifiedAt: "t" });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "new-file.yaml", isDirectory: false }), createMockContext());

    expect(response.status).toBe(201);
    const body = await response.json();
    expect(body.name).toBe("new-file.yaml");
    expect(vi.mocked(svc.writeContentFile)).toHaveBeenCalledWith(
      "test-ws",
      mockUser,
      "arena/projects/project-1/new-file.yaml",
      ""
    );
  });

  it("creates a new directory at root", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === "arena/projects/project-1") {
        return { path: relpath, entries: [] };
      }
      throw new svc.ContentApiError("not found", 404);
    });
    vi.mocked(svc.makeContentDir).mockResolvedValue({ path: "x", size: 0, modifiedAt: "t", directory: true });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "new-folder", isDirectory: true }), createMockContext());

    expect(response.status).toBe(201);
    const body = await response.json();
    expect(body.name).toBe("new-folder");
    expect(body.isDirectory).toBe(true);
  });

  it("returns 409 when file already exists", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    // Both the project-exists check and the target-exists check succeed.
    vi.mocked(svc.getContent).mockResolvedValue({ path: "x", entries: [] });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "existing.yaml", isDirectory: false }), createMockContext());

    expect(response.status).toBe(409);
  });

  it("creates a file with initial content", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === "arena/projects/project-1") {
        return { path: relpath, entries: [] };
      }
      throw new svc.ContentApiError("not found", 404);
    });
    vi.mocked(svc.writeContentFile).mockResolvedValue({ path: "x", size: 10, modifiedAt: "t" });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "new-file.yaml", isDirectory: false, content: "name: test" }), createMockContext());

    expect(response.status).toBe(201);
    expect(vi.mocked(svc.writeContentFile)).toHaveBeenCalledWith(
      "test-ws",
      mockUser,
      "arena/projects/project-1/new-file.yaml",
      "name: test"
    );
  });
});

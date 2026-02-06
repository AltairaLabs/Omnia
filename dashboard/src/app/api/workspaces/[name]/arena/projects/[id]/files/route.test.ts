/**
 * Tests for Arena project files listing API routes.
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

vi.mock("node:fs/promises", () => ({
  access: vi.fn(),
  stat: vi.fn(),
  readdir: vi.fn(),
  mkdir: vi.fn(),
  writeFile: vi.fn(),
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
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.access).mockRejectedValue(new Error("ENOENT"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns file tree", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.access).mockResolvedValue(undefined);
    // First call returns project contents, second call returns empty for subdirectory
    vi.mocked(fs.readdir)
      .mockResolvedValueOnce([
        { name: "config.arena.yaml", isDirectory: () => false },
        { name: "prompts", isDirectory: () => true },
      ] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>)
      .mockResolvedValueOnce([] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(fs.stat).mockResolvedValue({
      size: 100,
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.tree).toBeDefined();
    expect(Array.isArray(body.tree)).toBe(true);
  });

  it("returns empty tree for empty project", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.access).mockResolvedValue(undefined);
    vi.mocked(fs.readdir).mockResolvedValue([]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.tree).toEqual([]);
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
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.access).mockResolvedValue(undefined);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "../malicious", isDirectory: false }), createMockContext());

    expect(response.status).toBe(400);
  });

  it("creates a new file at root", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.access)
      .mockResolvedValueOnce(undefined) // project exists
      .mockRejectedValueOnce(new Error("ENOENT")); // file doesn't exist
    vi.mocked(fs.writeFile).mockResolvedValue(undefined);
    vi.mocked(fs.stat).mockResolvedValue({
      size: 0,
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "new-file.yaml", isDirectory: false }), createMockContext());

    expect(response.status).toBe(201);
    const body = await response.json();
    expect(body.name).toBe("new-file.yaml");
  });

  it("creates a new directory at root", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.access)
      .mockResolvedValueOnce(undefined) // project exists
      .mockRejectedValueOnce(new Error("ENOENT")); // directory doesn't exist
    vi.mocked(fs.mkdir).mockResolvedValue(undefined);
    vi.mocked(fs.stat).mockResolvedValue({
      size: 0,
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "new-folder", isDirectory: true }), createMockContext());

    expect(response.status).toBe(201);
    const body = await response.json();
    expect(body.name).toBe("new-folder");
    expect(body.isDirectory).toBe(true);
  });

  it("returns 409 when file already exists", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.access).mockResolvedValue(undefined); // Both project and file exist

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "existing.yaml", isDirectory: false }), createMockContext());

    expect(response.status).toBe(409);
  });

  it("creates a file with initial content", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.access)
      .mockResolvedValueOnce(undefined) // project exists
      .mockRejectedValueOnce(new Error("ENOENT")); // file doesn't exist
    vi.mocked(fs.writeFile).mockResolvedValue(undefined);
    vi.mocked(fs.stat).mockResolvedValue({
      size: 15,
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "new-file.yaml", isDirectory: false, content: "name: test" }), createMockContext());

    expect(response.status).toBe(201);
    expect(vi.mocked(fs.writeFile)).toHaveBeenCalledWith(
      expect.any(String),
      "name: test",
      "utf-8"
    );
  });
});

describe("GET /api/workspaces/[name]/arena/projects/[id]/files - file tree", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns tree with both files and directories sorted correctly", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.access).mockResolvedValue(undefined);
    // First call returns project contents with files and directories
    vi.mocked(fs.readdir)
      .mockResolvedValueOnce([
        { name: "zfile.yaml", isDirectory: () => false },
        { name: "adir", isDirectory: () => true },
        { name: "config.arena.yaml", isDirectory: () => false },
        { name: "prompts", isDirectory: () => true },
      ] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>)
      // Subsequent calls for subdirectories
      .mockResolvedValue([] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(fs.stat).mockResolvedValue({
      size: 100,
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.tree).toBeDefined();
    // Directories should come first, then files, both alphabetically
    expect(body.tree[0].name).toBe("adir");
    expect(body.tree[0].isDirectory).toBe(true);
    expect(body.tree[1].name).toBe("prompts");
    expect(body.tree[1].isDirectory).toBe(true);
    expect(body.tree[2].name).toBe("config.arena.yaml");
    expect(body.tree[2].isDirectory).toBe(false);
  });

  it("skips hidden files and directories", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.access).mockResolvedValue(undefined);
    vi.mocked(fs.readdir).mockResolvedValue([
      { name: ".hidden", isDirectory: () => true },
      { name: ".gitignore", isDirectory: () => false },
      { name: "visible.yaml", isDirectory: () => false },
    ] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(fs.stat).mockResolvedValue({
      size: 100,
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    // Only visible.yaml should be returned
    expect(body.tree.length).toBe(1);
    expect(body.tree[0].name).toBe("visible.yaml");
  });
});

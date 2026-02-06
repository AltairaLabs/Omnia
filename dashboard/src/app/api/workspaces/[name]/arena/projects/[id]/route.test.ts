/**
 * Tests for single Arena project API routes.
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
  readFile: vi.fn(),
  rmdir: vi.fn(),
  unlink: vi.fn(),
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

function createMockRequest(method = "GET"): NextRequest {
  const url = new URL("http://localhost:3000/api/workspaces/test-ws/arena/projects/project-1");
  return new NextRequest(url.toString(), { method });
}

function createMockContext(projectId = "project-1") {
  return {
    params: Promise.resolve({ name: "test-ws", id: projectId }),
  };
}

describe("GET /api/workspaces/[name]/arena/projects/[id]", () => {
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

  it("returns project with file tree", async () => {
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
    vi.mocked(fs.readFile).mockResolvedValue(Buffer.from("name: Test Project\ndescription: A test\ncreatedAt: 2024-01-01T00:00:00Z\nupdatedAt: 2024-01-01T00:00:00Z"));
    vi.mocked(fs.stat).mockResolvedValue({
      birthtime: new Date("2024-01-01T00:00:00Z"),
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);
    // First call returns project contents, second call returns empty for subdirectory
    vi.mocked(fs.readdir)
      .mockResolvedValueOnce([
        { name: "config.arena.yaml", isDirectory: () => false },
        { name: "prompts", isDirectory: () => true },
      ] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>)
      .mockResolvedValueOnce([] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.id).toBe("project-1");
    expect(body.name).toBe("Test Project");
    expect(body.tree).toBeDefined();
    expect(Array.isArray(body.tree)).toBe(true);
  });
});

describe("DELETE /api/workspaces/[name]/arena/projects/[id]", () => {
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

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 404 when project does not exist", async () => {
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
    vi.mocked(fs.access).mockRejectedValue(new Error("ENOENT"));

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(404);
  });

  it("deletes project successfully", async () => {
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
    vi.mocked(fs.readdir).mockResolvedValue([]);
    vi.mocked(fs.rmdir).mockResolvedValue(undefined);

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(204);
  });

  it("deletes project with files and subdirectories recursively", async () => {
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
    // First call: project root with file and directory
    vi.mocked(fs.readdir)
      .mockResolvedValueOnce([
        { name: "config.arena.yaml", isDirectory: () => false },
        { name: "prompts", isDirectory: () => true },
      ] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>)
      // Second call: prompts directory with a file
      .mockResolvedValueOnce([
        { name: "test.prompt.yaml", isDirectory: () => false },
      ] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(fs.unlink).mockResolvedValue(undefined);
    vi.mocked(fs.rmdir).mockResolvedValue(undefined);

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(204);
    expect(vi.mocked(fs.unlink)).toHaveBeenCalledTimes(2); // Both files
    expect(vi.mocked(fs.rmdir)).toHaveBeenCalledTimes(2); // Both directories
  });
});

describe("GET /api/workspaces/[name]/arena/projects/[id] - file tree", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns project with both files and directories in tree", async () => {
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
    vi.mocked(fs.readFile).mockResolvedValue(Buffer.from("name: Test Project\ndescription: A test"));
    // First call: project root
    vi.mocked(fs.readdir)
      .mockResolvedValueOnce([
        { name: "config.arena.yaml", isDirectory: () => false },
        { name: "prompts", isDirectory: () => true },
        { name: ".hidden", isDirectory: () => true }, // Should be skipped
      ] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>)
      // Second call: prompts directory
      .mockResolvedValueOnce([
        { name: "test.prompt.yaml", isDirectory: () => false },
      ] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(fs.stat).mockResolvedValue({
      birthtime: new Date("2024-01-01T00:00:00Z"),
      mtime: new Date("2024-01-01T00:00:00Z"),
      size: 100,
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.tree).toBeDefined();
    // Should have prompts directory and config file (hidden is skipped)
    expect(body.tree.length).toBe(2);
  });

  it("returns project when config file is missing", async () => {
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
    // Config file doesn't exist
    vi.mocked(fs.readFile).mockRejectedValue(new Error("ENOENT"));
    vi.mocked(fs.readdir).mockResolvedValue([]);
    vi.mocked(fs.stat).mockResolvedValue({
      birthtime: new Date("2024-01-01T00:00:00Z"),
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    // Should use projectId as name
    expect(body.name).toBe("project-1");
  });
});

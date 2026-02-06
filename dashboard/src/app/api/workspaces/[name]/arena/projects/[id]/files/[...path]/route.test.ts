/**
 * Tests for Arena project file content API routes.
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
  writeFile: vi.fn(),
  mkdir: vi.fn(),
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

function createMockRequest(method = "GET", body?: unknown): NextRequest {
  const url = new URL("http://localhost:3000/api/workspaces/test-ws/arena/projects/project-1/files/test.yaml");
  if (body) {
    return new NextRequest(url.toString(), {
      method,
      body: JSON.stringify(body),
      headers: { "Content-Type": "application/json" },
    });
  }
  return new NextRequest(url.toString(), { method });
}

function createMockContext(pathSegments: string[] = ["test.yaml"]) {
  return {
    params: Promise.resolve({ name: "test-ws", id: "project-1", path: pathSegments }),
  };
}

describe("GET /api/workspaces/[name]/arena/projects/[id]/files/[...path]", () => {
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

  it("returns 404 when file does not exist", async () => {
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

  it("returns file content for text file", async () => {
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
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => false,
      size: 100,
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);
    vi.mocked(fs.readFile).mockResolvedValue(Buffer.from("name: test\nversion: 1.0"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.path).toBe("test.yaml");
    expect(body.content).toBe("name: test\nversion: 1.0");
    expect(body.encoding).toBe("utf-8");
  });

  it("returns 400 for path traversal attempts", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext(["..", "..", "etc", "passwd"]));

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("path traversal");
  });

  it("returns 400 when trying to get content of a directory", async () => {
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
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => true,
      size: 0,
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext(["prompts"]));

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("directory");
  });

  it("returns 400 when file is too large", async () => {
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
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => false,
      size: 20000000, // 20MB, over the 10MB limit
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext(["large.bin"]));

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("too large");
  });

  it("returns binary file content as base64", async () => {
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
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => false,
      size: 100,
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);
    vi.mocked(fs.readFile).mockResolvedValue(Buffer.from([0x00, 0x01, 0x02, 0x03]));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext(["image.png"]));

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.encoding).toBe("base64");
  });
});

describe("PUT /api/workspaces/[name]/arena/projects/[id]/files/[...path]", () => {
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

    const { PUT } = await import("./route");
    const response = await PUT(createMockRequest("PUT", { content: "test" }), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 404 when file does not exist", async () => {
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
    vi.mocked(fs.stat).mockRejectedValue(new Error("ENOENT"));

    const { PUT } = await import("./route");
    const response = await PUT(createMockRequest("PUT", { content: "test" }), createMockContext());

    expect(response.status).toBe(404);
  });

  it("updates file content", async () => {
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
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => false,
      size: 100,
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);
    vi.mocked(fs.writeFile).mockResolvedValue(undefined);

    const { PUT } = await import("./route");
    const response = await PUT(createMockRequest("PUT", { content: "name: updated" }), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.path).toBe("test.yaml");
    expect(vi.mocked(fs.writeFile)).toHaveBeenCalled();
  });

  it("returns 400 when content is missing", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const { PUT } = await import("./route");
    const response = await PUT(createMockRequest("PUT", {}), createMockContext());

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("Content is required");
  });

  it("returns 400 when trying to update a directory", async () => {
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
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => true,
      size: 0,
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);

    const { PUT } = await import("./route");
    const response = await PUT(createMockRequest("PUT", { content: "test" }), createMockContext(["prompts"]));

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("directory");
  });

  it("returns 400 for path traversal on PUT", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const { PUT } = await import("./route");
    const response = await PUT(createMockRequest("PUT", { content: "test" }), createMockContext(["..", "etc", "passwd"]));

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("path traversal");
  });
});

describe("DELETE /api/workspaces/[name]/arena/projects/[id]/files/[...path]", () => {
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

  it("returns 404 when file does not exist", async () => {
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

  it("deletes a file", async () => {
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
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => false,
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);
    vi.mocked(fs.unlink).mockResolvedValue(undefined);

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(204);
    expect(vi.mocked(fs.unlink)).toHaveBeenCalled();
  });

  it("returns 400 when trying to delete config.arena.yaml", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext(["config.arena.yaml"]));

    expect(response.status).toBe(400);
  });

  it("deletes a directory recursively", async () => {
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
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => true,
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);
    vi.mocked(fs.readdir)
      .mockResolvedValueOnce([
        { name: "file1.txt", isDirectory: () => false },
        { name: "subdir", isDirectory: () => true },
      ] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>)
      .mockResolvedValueOnce([
        { name: "file2.txt", isDirectory: () => false },
      ] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(fs.unlink).mockResolvedValue(undefined);
    vi.mocked(fs.rmdir).mockResolvedValue(undefined);

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext(["prompts"]));

    expect(response.status).toBe(204);
    expect(vi.mocked(fs.unlink)).toHaveBeenCalled();
    expect(vi.mocked(fs.rmdir)).toHaveBeenCalled();
  });

  it("returns 400 for path traversal on DELETE", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext(["..", "etc", "passwd"]));

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("path traversal");
  });
});

describe("POST /api/workspaces/[name]/arena/projects/[id]/files/[...path]", () => {
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
    const response = await POST(createMockRequest("POST", { name: "new.yaml", isDirectory: false }), createMockContext(["prompts"]));

    expect(response.status).toBe(403);
  });

  it("creates a file in subdirectory", async () => {
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
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => true,
      size: 0,
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);
    vi.mocked(fs.access).mockRejectedValueOnce(new Error("ENOENT")); // File doesn't exist
    vi.mocked(fs.writeFile).mockResolvedValue(undefined);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "new.yaml", isDirectory: false }), createMockContext(["prompts"]));

    expect(response.status).toBe(201);
    const body = await response.json();
    expect(body.path).toBe("prompts/new.yaml");
  });

  it("creates a directory in subdirectory", async () => {
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
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => true,
      size: 0,
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);
    vi.mocked(fs.access).mockRejectedValueOnce(new Error("ENOENT")); // Directory doesn't exist
    vi.mocked(fs.mkdir).mockResolvedValue(undefined);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "subfolder", isDirectory: true }), createMockContext(["prompts"]));

    expect(response.status).toBe(201);
    const body = await response.json();
    expect(body.isDirectory).toBe(true);
  });

  it("returns 400 for invalid filename", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: ".hidden", isDirectory: false }), createMockContext(["prompts"]));

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("Invalid filename");
  });

  it("returns 400 when parent path is not a directory", async () => {
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
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => false, // Parent is a file, not a directory
      size: 100,
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "new.yaml", isDirectory: false }), createMockContext(["config.arena.yaml"]));

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("not a directory");
  });

  it("returns 404 when parent directory does not exist", async () => {
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
    vi.mocked(fs.stat).mockRejectedValue(new Error("ENOENT"));

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "new.yaml", isDirectory: false }), createMockContext(["nonexistent"]));

    expect(response.status).toBe(404);
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
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => true,
      size: 0,
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);
    vi.mocked(fs.access).mockResolvedValue(undefined); // File already exists

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "existing.yaml", isDirectory: false }), createMockContext(["prompts"]));

    expect(response.status).toBe(409);
  });

  it("returns 400 for path traversal on POST", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "test.yaml", isDirectory: false }), createMockContext(["..", "etc"]));

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("path traversal");
  });

  it("creates file with initial content", async () => {
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
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => true,
      size: 0,
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);
    vi.mocked(fs.access).mockRejectedValueOnce(new Error("ENOENT"));
    vi.mocked(fs.writeFile).mockResolvedValue(undefined);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "new.yaml", isDirectory: false, content: "name: test" }), createMockContext(["prompts"]));

    expect(response.status).toBe(201);
    expect(vi.mocked(fs.writeFile)).toHaveBeenCalledWith(
      expect.any(String),
      "name: test",
      "utf-8"
    );
  });
});

/**
 * Tests for Arena project file content API routes.
 *
 * The routes now call the operator content API via content-api-service instead
 * of reading the NFS mount (mock-to-contract: shapes match the Go
 * content.Listing / content.FileContent / content.WriteResult json tags).
 * Path-confinement, max-size and text/binary encoding are operator-side and
 * surface here as pass-through statuses (400/413).
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
    deleteContent: vi.fn(),
  };
});

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
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns file content for text file", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockResolvedValue({
      path: "arena/projects/project-1/test.yaml",
      content: "name: test\nversion: 1.0",
      encoding: "utf-8",
      size: 23,
      modifiedAt: "2024-01-01T00:00:00Z",
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.path).toBe("test.yaml");
    expect(body.content).toBe("name: test\nversion: 1.0");
    expect(body.encoding).toBe("utf-8");
  });

  it("passes through 400 for an operator-rejected (traversal) path", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("invalid path", 400));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext(["..", "..", "etc", "passwd"]));

    expect(response.status).toBe(400);
  });

  it("returns 400 when trying to get content of a directory", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockResolvedValue({ path: "arena/projects/project-1/prompts", entries: [] });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext(["prompts"]));

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("directory");
  });

  it("passes through 413 when the operator rejects a too-large file", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("file too large", 413));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext(["large.bin"]));

    expect(response.status).toBe(413);
  });

  it("returns binary file content as base64 (operator-encoded)", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockResolvedValue({
      path: "arena/projects/project-1/image.png",
      content: "AAECAw==",
      encoding: "base64",
      size: 4,
      modifiedAt: "2024-01-01T00:00:00Z",
    });

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
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { PUT } = await import("./route");
    const response = await PUT(createMockRequest("PUT", { content: "test" }), createMockContext());

    expect(response.status).toBe(404);
  });

  it("updates file content", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockResolvedValue({
      path: "arena/projects/project-1/test.yaml",
      content: "old",
      encoding: "utf-8",
      size: 3,
      modifiedAt: "t",
    });
    vi.mocked(svc.writeContentFile).mockResolvedValue({ path: "x", size: 12, modifiedAt: "t2" });

    const { PUT } = await import("./route");
    const response = await PUT(createMockRequest("PUT", { content: "name: updated" }), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.path).toBe("test.yaml");
    expect(vi.mocked(svc.writeContentFile)).toHaveBeenCalledWith(
      "test-ws",
      mockUser,
      "arena/projects/project-1/test.yaml",
      "name: updated"
    );
  });

  it("returns 400 when content is missing", async () => {
    await grantWorkspace("editor");

    const { PUT } = await import("./route");
    const response = await PUT(createMockRequest("PUT", {}), createMockContext());

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("Content is required");
  });

  it("returns 400 when trying to update a directory", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockResolvedValue({ path: "arena/projects/project-1/prompts", entries: [] });

    const { PUT } = await import("./route");
    const response = await PUT(createMockRequest("PUT", { content: "test" }), createMockContext(["prompts"]));

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("directory");
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
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(404);
  });

  it("deletes a file", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockResolvedValue({
      path: "arena/projects/project-1/test.yaml",
      content: "x",
      encoding: "utf-8",
      size: 1,
      modifiedAt: "t",
    });
    vi.mocked(svc.deleteContent).mockResolvedValue(undefined);

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(204);
    expect(vi.mocked(svc.deleteContent)).toHaveBeenCalledWith(
      "test-ws",
      mockUser,
      "arena/projects/project-1/test.yaml"
    );
  });

  it("returns 400 when trying to delete config.arena.yaml", async () => {
    await grantWorkspace("editor");

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext(["config.arena.yaml"]));

    expect(response.status).toBe(400);
  });

  it("deletes a directory (recursive delete is operator-side)", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockResolvedValue({ path: "arena/projects/project-1/prompts", entries: [] });
    vi.mocked(svc.deleteContent).mockResolvedValue(undefined);

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext(["prompts"]));

    expect(response.status).toBe(204);
    expect(vi.mocked(svc.deleteContent)).toHaveBeenCalled();
  });

  it("passes through 400 for an operator-rejected (traversal) path", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("invalid path", 400));

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext(["..", "etc", "passwd"]));

    expect(response.status).toBe(400);
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
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === "arena/projects/project-1/prompts") {
        return { path: relpath, entries: [] }; // parent is a directory
      }
      throw new svc.ContentApiError("not found", 404); // target does not exist
    });
    vi.mocked(svc.writeContentFile).mockResolvedValue({ path: "x", size: 0, modifiedAt: "t" });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "new.yaml", isDirectory: false }), createMockContext(["prompts"]));

    expect(response.status).toBe(201);
    const body = await response.json();
    expect(body.path).toBe("prompts/new.yaml");
  });

  it("creates a directory in subdirectory", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === "arena/projects/project-1/prompts") {
        return { path: relpath, entries: [] };
      }
      throw new svc.ContentApiError("not found", 404);
    });
    vi.mocked(svc.makeContentDir).mockResolvedValue({ path: "x", size: 0, modifiedAt: "t", directory: true });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "subfolder", isDirectory: true }), createMockContext(["prompts"]));

    expect(response.status).toBe(201);
    const body = await response.json();
    expect(body.isDirectory).toBe(true);
  });

  it("returns 400 for invalid filename", async () => {
    await grantWorkspace("editor");

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: ".hidden", isDirectory: false }), createMockContext(["prompts"]));

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("Invalid filename");
  });

  it("returns 400 when parent path is not a directory", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    // Parent resolves to a file, not a directory.
    vi.mocked(svc.getContent).mockResolvedValue({
      path: "arena/projects/project-1/config.arena.yaml",
      content: "x",
      encoding: "utf-8",
      size: 1,
      modifiedAt: "t",
    });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "new.yaml", isDirectory: false }), createMockContext(["config.arena.yaml"]));

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("not a directory");
  });

  it("returns 404 when parent directory does not exist", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "new.yaml", isDirectory: false }), createMockContext(["nonexistent"]));

    expect(response.status).toBe(404);
  });

  it("returns 409 when file already exists", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    // Parent is a directory AND the target already exists.
    vi.mocked(svc.getContent).mockResolvedValue({ path: "x", entries: [] });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "existing.yaml", isDirectory: false }), createMockContext(["prompts"]));

    expect(response.status).toBe(409);
  });

  it("creates file with initial content", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === "arena/projects/project-1/prompts") {
        return { path: relpath, entries: [] };
      }
      throw new svc.ContentApiError("not found", 404);
    });
    vi.mocked(svc.writeContentFile).mockResolvedValue({ path: "x", size: 10, modifiedAt: "t" });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "new.yaml", isDirectory: false, content: "name: test" }), createMockContext(["prompts"]));

    expect(response.status).toBe(201);
    expect(vi.mocked(svc.writeContentFile)).toHaveBeenCalledWith(
      "test-ws",
      mockUser,
      "arena/projects/project-1/prompts/new.yaml",
      "name: test"
    );
  });
});

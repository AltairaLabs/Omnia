/**
 * Tests for single Arena project API routes.
 *
 * The routes now call the operator content API via content-api-service and
 * content-tree instead of reading the NFS mount, so these mock those services
 * (mock-to-contract: shapes match the Go content.Listing / content.FileContent
 * json tags).
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest, NextResponse } from "next/server";

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
    deleteContent: vi.fn(),
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

function createMockRequest(method = "GET"): NextRequest {
  const url = new URL("http://localhost:3000/api/workspaces/test-ws/arena/projects/project-1");
  return new NextRequest(url.toString(), { method });
}

function createMockContext(projectId = "project-1") {
  return {
    params: Promise.resolve({ name: "test-ws", id: projectId }),
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
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns project with file tree", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    const { listContentTree } = await import("@/lib/data/content-tree");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === "arena/projects/project-1") {
        return { path: relpath, entries: [] };
      }
      if (relpath.endsWith("config.arena.yaml")) {
        return {
          path: relpath,
          content: "name: Test Project\ndescription: A test\ncreatedAt: 2024-01-01T00:00:00Z\nupdatedAt: 2024-01-01T00:00:00Z",
          encoding: "utf-8",
          size: 80,
          modifiedAt: "2024-01-01T00:00:00Z",
        };
      }
      throw new svc.ContentApiError("not found", 404);
    });
    vi.mocked(listContentTree).mockResolvedValue([
      { name: "config.arena.yaml", path: "config.arena.yaml", isDirectory: false, size: 80, modifiedAt: "2024-01-01T00:00:00Z" },
      { name: "prompts", path: "prompts", isDirectory: true, modifiedAt: "2024-01-01T00:00:00Z", children: [] },
    ]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.id).toBe("project-1");
    expect(body.name).toBe("Test Project");
    expect(body.tree).toBeDefined();
    expect(Array.isArray(body.tree)).toBe(true);
    // Directories sort before files.
    expect(body.tree[0].name).toBe("prompts");
    expect(body.tree[1].name).toBe("config.arena.yaml");
  });

  it("returns project using projectId as name when config is missing", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    const { listContentTree } = await import("@/lib/data/content-tree");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === "arena/projects/project-1") {
        return { path: relpath, entries: [] };
      }
      throw new svc.ContentApiError("not found", 404);
    });
    vi.mocked(listContentTree).mockResolvedValue([]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.name).toBe("project-1");
    expect(body.tree).toEqual([]);
  });

  it("recurses into directories and enriches provider files", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    const { listContentTree } = await import("@/lib/data/content-tree");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === "arena/projects/project-1") return { path: relpath, entries: [] };
      if (relpath.endsWith("config.arena.yaml")) {
        return { path: relpath, content: "name: P", encoding: "utf-8", size: 6, modifiedAt: "t" };
      }
      if (relpath.endsWith(".provider.yaml")) {
        return { path: relpath, content: "model: gpt-4", encoding: "utf-8", size: 12, modifiedAt: "t" };
      }
      throw new svc.ContentApiError("not found", 404);
    });
    vi.mocked(listContentTree).mockResolvedValue([
      {
        name: "providers",
        path: "providers",
        isDirectory: true,
        modifiedAt: "t",
        children: [
          { name: "openai.provider.yaml", path: "providers/openai.provider.yaml", isDirectory: false, size: 12, modifiedAt: "t" },
        ],
      },
      { name: "openai.provider.yaml", path: "openai.provider.yaml", isDirectory: false, size: 12, modifiedAt: "t" },
    ]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.tree[0].name).toBe("providers");
    expect(body.tree[0].children[0].type).toBe("provider");
    expect(body.tree[1].type).toBe("provider");
  });

  it("returns the validateWorkspace error when the workspace is invalid", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: false,
      response: NextResponse.json({ error: "nope" }, { status: 404 }),
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());
    expect(response.status).toBe(404);
  });

  it("passes through a non-404 content error", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("boom", 500));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());
    expect(response.status).toBe(500);
  });

  it("rethrows a non-404 error from the config read", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    const { listContentTree } = await import("@/lib/data/content-tree");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === "arena/projects/project-1") return { path: relpath, entries: [] };
      throw new svc.ContentApiError("boom", 500);
    });
    vi.mocked(listContentTree).mockResolvedValue([]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());
    expect(response.status).toBe(500);
  });

  it("returns 500 via handleK8sError on a non-content error", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    const { listContentTree } = await import("@/lib/data/content-tree");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === "arena/projects/project-1") return { path: relpath, entries: [] };
      throw new svc.ContentApiError("not found", 404);
    });
    vi.mocked(listContentTree).mockRejectedValue(new Error("k8s boom"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());
    expect(response.status).toBe(500);
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
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(404);
  });

  it("deletes project successfully (recursive delete is operator-side)", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockResolvedValue({ path: "arena/projects/project-1", entries: [] });
    vi.mocked(svc.deleteContent).mockResolvedValue(undefined);

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(204);
    expect(vi.mocked(svc.deleteContent)).toHaveBeenCalledWith("test-ws", mockUser, "arena/projects/project-1");
  });

  it("returns the validateWorkspace error when the workspace is invalid", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: false,
      response: NextResponse.json({ error: "nope" }, { status: 404 }),
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());
    expect(response.status).toBe(404);
  });

  it("passes through a non-404 content error", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("boom", 500));

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());
    expect(response.status).toBe(500);
  });
});

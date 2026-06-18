/**
 * Tests for Arena projects API routes.
 *
 * The routes now call the operator content API via content-api-service instead
 * of reading the NFS mount, so these mock that service (mock-to-contract: the
 * mocked shapes match the Go content.Listing / content.FileContent json tags).
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
  const url = new URL("http://localhost:3000/api/workspaces/test-ws/arena/projects");
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
    params: Promise.resolve({ name: "test-ws" }),
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

describe("GET /api/workspaces/[name]/arena/projects", () => {
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

  it("returns 404 when workspace is not found", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { NextResponse } = await import("next/server");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: false,
      response: NextResponse.json({ error: "Not Found", message: "Workspace not found" }, { status: 404 }),
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns empty list when projects directory does not exist", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.projects).toEqual([]);
  });

  it("returns list of projects sorted by updatedAt (most recent first)", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === "arena/projects") {
        return {
          path: relpath,
          entries: [
            { name: "project-1", type: "directory", size: 0, modifiedAt: "2024-01-01T00:00:00Z" },
            { name: "project-2", type: "directory", size: 0, modifiedAt: "2024-01-01T00:00:00Z" },
            { name: ".hidden", type: "directory", size: 0, modifiedAt: "2024-01-01T00:00:00Z" }, // skipped
            { name: "file.txt", type: "file", size: 5, modifiedAt: "2024-01-01T00:00:00Z" }, // skipped
          ],
        };
      }
      if (relpath.endsWith("project-1/config.arena.yaml")) {
        return {
          path: relpath,
          content: "name: Project One\nupdatedAt: 2024-01-01T00:00:00Z",
          encoding: "utf-8",
          size: 40,
          modifiedAt: "2024-01-01T00:00:00Z",
        };
      }
      if (relpath.endsWith("project-2/config.arena.yaml")) {
        return {
          path: relpath,
          content: "name: Project Two\nupdatedAt: 2024-06-01T00:00:00Z",
          encoding: "utf-8",
          size: 40,
          modifiedAt: "2024-06-01T00:00:00Z",
        };
      }
      throw new svc.ContentApiError("not found", 404);
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.projects).toHaveLength(2);
    // project-2 (2024-06) sorts before project-1 (2024-01).
    expect(body.projects[0].name).toBe("Project Two");
    expect(body.projects[1].name).toBe("Project One");
  });

  it("falls back to project id as name when config is missing", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === "arena/projects") {
        return {
          path: relpath,
          entries: [
            { name: "orphan", type: "directory", size: 0, modifiedAt: "2024-01-01T00:00:00Z" },
          ],
        };
      }
      throw new svc.ContentApiError("not found", 404);
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.projects[0].name).toBe("orphan");
  });

  it("passes through the operator status (500) on an unexpected content error", async () => {
    await grantWorkspace("viewer");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("boom", 500));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(500);
  });
});

describe("POST /api/workspaces/[name]/arena/projects", () => {
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
    const response = await POST(createMockRequest("POST", { name: "New Project" }), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 400 when name is missing", async () => {
    await grantWorkspace("editor");

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", {}), createMockContext());

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("name");
  });

  it("creates a new project successfully", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.makeContentDir).mockResolvedValue({ path: "x", size: 0, modifiedAt: "t", directory: true });
    vi.mocked(svc.writeContentFile).mockResolvedValue({ path: "x", size: 100, modifiedAt: "t" });

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest("POST", { name: "My New Project", description: "A test project" }),
      createMockContext()
    );

    expect(response.status).toBe(201);
    const body = await response.json();
    expect(body.name).toBe("My New Project");
    expect(body.description).toBe("A test project");
    expect(body.id).toBeDefined();
    expect(vi.mocked(svc.makeContentDir)).toHaveBeenCalled();
    expect(vi.mocked(svc.writeContentFile)).toHaveBeenCalled();
  });

  it("writes content under the workspace-relative arena/projects path", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.makeContentDir).mockResolvedValue({ path: "x", size: 0, modifiedAt: "t", directory: true });
    vi.mocked(svc.writeContentFile).mockResolvedValue({ path: "x", size: 100, modifiedAt: "t" });

    const { POST } = await import("./route");
    await POST(createMockRequest("POST", { name: "Test Project" }), createMockContext());

    // The relpath is workspace-relative (the operator prepends ws/namespace).
    const mkdirCalls = vi.mocked(svc.makeContentDir).mock.calls;
    expect(mkdirCalls.length).toBe(4);
    expect(mkdirCalls[0][2]).toMatch(/^arena\/projects\/test-project-[a-z0-9]+\/prompts$/);
    const writeCall = vi.mocked(svc.writeContentFile).mock.calls[0];
    expect(writeCall[2]).toMatch(/^arena\/projects\/test-project-[a-z0-9]+\/config\.arena\.yaml$/);
  });

  it("passes through the operator status on a write failure", async () => {
    await grantWorkspace("editor");
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.makeContentDir).mockResolvedValue({ path: "x", size: 0, modifiedAt: "t", directory: true });
    vi.mocked(svc.writeContentFile).mockRejectedValue(new svc.ContentApiError("payload too large", 413));

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { name: "Test Project" }), createMockContext());

    expect(response.status).toBe(413);
  });
});

/**
 * Tests for Arena projects API routes.
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

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.projects).toEqual([]);
  });

  it("returns list of projects", async () => {
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
      { name: "project-1", isDirectory: () => true },
      { name: "project-2", isDirectory: () => true },
      { name: ".hidden", isDirectory: () => true }, // Should be skipped
      { name: "file.txt", isDirectory: () => false }, // Should be skipped
    ] as unknown[] as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(fs.readFile).mockResolvedValue(Buffer.from("name: Test Project\ncreatedAt: 2024-01-01T00:00:00Z\nupdatedAt: 2024-01-01T00:00:00Z"));
    vi.mocked(fs.stat).mockResolvedValue({
      birthtime: new Date("2024-01-01T00:00:00Z"),
      mtime: new Date("2024-01-01T00:00:00Z"),
    } as unknown as Awaited<ReturnType<typeof fs.stat>>);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.projects).toHaveLength(2);
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
    const response = await POST(createMockRequest("POST", {}), createMockContext());

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("name");
  });

  it("creates a new project successfully", async () => {
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
    vi.mocked(fs.mkdir).mockResolvedValue(undefined);
    vi.mocked(fs.writeFile).mockResolvedValue(undefined);

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
    expect(vi.mocked(fs.mkdir)).toHaveBeenCalled();
    expect(vi.mocked(fs.writeFile)).toHaveBeenCalled();
  });

  it("constructs correct path using workspace name and namespace", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace, // workspace name: test-ws, namespace: test-ns
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.mkdir).mockResolvedValue(undefined);
    vi.mocked(fs.writeFile).mockResolvedValue(undefined);

    const { POST } = await import("./route");
    await POST(
      createMockRequest("POST", { name: "Test Project" }),
      createMockContext()
    );

    // Verify mkdir was called with correct path structure:
    // /workspace-content/{workspaceName}/{namespace}/arena/projects/{projectId}
    const mkdirCalls = vi.mocked(fs.mkdir).mock.calls;
    expect(mkdirCalls.length).toBeGreaterThan(0);

    // First mkdir call should be for the projects directory
    const projectsPath = mkdirCalls[0][0] as string;
    expect(projectsPath).toContain("/workspace-content/test-ws/test-ns/arena/projects");
  });

  it("handles EACCES (permission denied) error gracefully", async () => {
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

    // Simulate permission denied error
    const permissionError = new Error("EACCES: permission denied, mkdir '/workspace-content/test-ws'");
    (permissionError as NodeJS.ErrnoException).code = "EACCES";
    vi.mocked(fs.mkdir).mockRejectedValue(permissionError);

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest("POST", { name: "Test Project" }),
      createMockContext()
    );

    expect(response.status).toBe(500);
    const body = await response.json();
    expect(body.error).toBe("Internal Server Error");
  });

  it("handles ESTALE (stale NFS handle) error gracefully", async () => {
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

    // Simulate stale NFS handle error (error code -116 on Linux)
    const staleError = new Error("Unknown system error -116");
    (staleError as NodeJS.ErrnoException).code = "UNKNOWN";
    (staleError as NodeJS.ErrnoException).errno = -116;
    vi.mocked(fs.mkdir).mockRejectedValue(staleError);

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest("POST", { name: "Test Project" }),
      createMockContext()
    );

    expect(response.status).toBe(500);
    const body = await response.json();
    expect(body.error).toBe("Internal Server Error");
  });

  it("handles workspace where namespace equals workspace name", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    // Workspace where name and namespace are the same (like dev-agents)
    const sameNameWorkspace = {
      metadata: { name: "dev-agents", namespace: "omnia-system" },
      spec: { namespace: { name: "dev-agents" } },
    };

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: sameNameWorkspace,
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(fs.mkdir).mockResolvedValue(undefined);
    vi.mocked(fs.writeFile).mockResolvedValue(undefined);

    // Need to use the correct workspace name in the request
    const customContext = {
      params: Promise.resolve({ name: "dev-agents" }),
    };

    const { POST } = await import("./route");
    await POST(
      createMockRequest("POST", { name: "Test Project" }),
      customContext
    );

    // Verify the path has workspace name and namespace (both "dev-agents")
    const mkdirCalls = vi.mocked(fs.mkdir).mock.calls;
    const projectsPath = mkdirCalls[0][0] as string;

    // Path should be /workspace-content/dev-agents/dev-agents/arena/projects/...
    // This is the expected behavior - workspace name and namespace are both in the path
    expect(projectsPath).toContain("/workspace-content/dev-agents/dev-agents/arena/projects");
  });
});

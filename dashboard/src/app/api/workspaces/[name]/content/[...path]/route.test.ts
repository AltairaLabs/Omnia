/**
 * Tests for workspace content filesystem API routes.
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
  };
});

vi.mock("node:fs/promises", () => ({
  access: vi.fn(),
  stat: vi.fn(),
  readdir: vi.fn(),
  readFile: vi.fn(),
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

function createMockRequest(queryParams?: Record<string, string>): NextRequest {
  const url = new URL("http://localhost:3000/api/workspaces/test-ws/content/arena/test-config");
  if (queryParams) {
    for (const [key, value] of Object.entries(queryParams)) {
      url.searchParams.set(key, value);
    }
  }
  return new NextRequest(url.toString(), { method: "GET" });
}

function createMockContext(pathSegments: string[] = ["arena", "test-config"]) {
  return {
    params: Promise.resolve({ name: "test-ws", path: pathSegments }),
  };
}

describe("GET /api/workspaces/[name]/content/[...path]", () => {
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
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(null);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.error).toBe("Not Found");
  });

  it("returns 404 when path does not exist", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as any);
    vi.mocked(fs.access).mockRejectedValue(new Error("ENOENT"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.message).toContain("Path not found");
  });

  it("returns 400 for path traversal attempts", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as any);

    const { GET } = await import("./route");
    const response = await GET(
      createMockRequest(),
      createMockContext(["..", "..", "etc", "passwd"])
    );

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("path traversal");
  });

  it("returns directory listing for directory path", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as any);
    vi.mocked(fs.access).mockResolvedValue(undefined);
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => true,
      isFile: () => false,
      size: 4096,
      mtime: new Date("2025-01-01T00:00:00Z"),
    } as any);
    vi.mocked(fs.readdir).mockResolvedValue([
      { name: "config.yaml", isDirectory: () => false, isFile: () => true },
      { name: "prompts", isDirectory: () => true, isFile: () => false },
    ] as any);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.path).toBe("arena/test-config");
    expect(body.files).toBeDefined();
    expect(body.directories).toBeDefined();
  });

  it("returns 400 when requesting file=true on a directory", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as any);
    vi.mocked(fs.access).mockResolvedValue(undefined);
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => true,
      isFile: () => false,
      size: 4096,
      mtime: new Date("2025-01-01T00:00:00Z"),
    } as any);

    const { GET } = await import("./route");
    const response = await GET(
      createMockRequest({ file: "true" }),
      createMockContext()
    );

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("Cannot return file content for a directory");
  });

  it("returns file info for file path without file=true", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as any);
    vi.mocked(fs.access).mockResolvedValue(undefined);
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => false,
      isFile: () => true,
      size: 1024,
      mtime: new Date("2025-01-01T00:00:00Z"),
    } as any);

    const { GET } = await import("./route");
    const response = await GET(
      createMockRequest(),
      createMockContext(["arena", "test-config", "config.yaml"])
    );

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.name).toBe("config.yaml");
    expect(body.type).toBe("file");
    expect(body.size).toBe(1024);
  });

  it("returns file content for file path with file=true", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as any);
    vi.mocked(fs.access).mockResolvedValue(undefined);
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => false,
      isFile: () => true,
      size: 100,
      mtime: new Date("2025-01-01T00:00:00Z"),
    } as any);
    vi.mocked(fs.readFile).mockResolvedValue(Buffer.from("apiVersion: v1\nkind: Arena"));

    const { GET } = await import("./route");
    const response = await GET(
      createMockRequest({ file: "true" }),
      createMockContext(["arena", "test-config", "config.yaml"])
    );

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.content).toBe("apiVersion: v1\nkind: Arena");
    expect(body.encoding).toBe("utf-8");
  });

  it("returns 400 for files that are too large", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as any);
    vi.mocked(fs.access).mockResolvedValue(undefined);
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => false,
      isFile: () => true,
      size: 20 * 1024 * 1024, // 20MB - over the 10MB default limit
      mtime: new Date("2025-01-01T00:00:00Z"),
    } as any);

    const { GET } = await import("./route");
    const response = await GET(
      createMockRequest({ file: "true" }),
      createMockContext(["arena", "test-config", "large-file.bin"])
    );

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("File too large");
  });

  it("returns base64 encoding for binary files", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as any);
    vi.mocked(fs.access).mockResolvedValue(undefined);
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => false,
      isFile: () => true,
      size: 100,
      mtime: new Date("2025-01-01T00:00:00Z"),
    } as any);
    // Create binary content with null bytes
    const binaryContent = Buffer.alloc(100);
    binaryContent[50] = 0; // null byte indicates binary
    vi.mocked(fs.readFile).mockResolvedValue(binaryContent);

    const { GET } = await import("./route");
    const response = await GET(
      createMockRequest({ file: "true" }),
      createMockContext(["arena", "test-config", "image.png"])
    );

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.encoding).toBe("base64");
  });

  it("handles version parameter for arena content", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const fs = await import("node:fs/promises");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as any);
    // All fs.access calls succeed
    vi.mocked(fs.access).mockResolvedValue(undefined);
    vi.mocked(fs.stat).mockResolvedValue({
      isDirectory: () => true,
      isFile: () => false,
      size: 4096,
      mtime: new Date("2025-01-01T00:00:00Z"),
    } as any);
    vi.mocked(fs.readdir).mockResolvedValue([]);
    // Mock readFile for HEAD file
    vi.mocked(fs.readFile).mockResolvedValue(Buffer.from("abc123"));

    const { GET } = await import("./route");
    const response = await GET(
      createMockRequest({ version: "latest" }),
      createMockContext(["arena", "test-config"])
    );

    // Should succeed with empty directory listing
    expect(response.status).toBe(200);
  });
});

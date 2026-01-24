/**
 * Tests for Arena source versions API route.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest, NextResponse } from "next/server";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

// Create mock fs functions
const mockExistsSync = vi.fn();
const mockReadFileSync = vi.fn();
const mockWriteFileSync = vi.fn();
const mockReaddirSync = vi.fn();
const mockStatSync = vi.fn();
const mockMkdirSync = vi.fn();

type RouteHandler = (
  request: NextRequest,
  context: { params: Promise<{ name: string; sourceName: string }> },
  access: WorkspaceAccess,
  user: User
) => Promise<NextResponse>;

// Mock dependencies before imports
vi.mock("@/lib/auth/workspace-guard", () => ({
  withWorkspaceAccess: vi.fn((_role: string, handler: RouteHandler) => {
    return async (request: NextRequest, context: { params: Promise<{ name: string; sourceName: string }> }) => {
      const mockAccess: WorkspaceAccess = {
        granted: true,
        role: "editor",
        permissions: { read: true, write: true, delete: false, manageMembers: false },
      };
      const mockUser: User = {
        id: "testuser",
        provider: "oauth" as const,
        username: "testuser",
        email: "test@example.com",
        groups: [],
        role: "admin" as const,
      };
      return handler(request, context, mockAccess, mockUser);
    };
  }),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", () => ({
  getWorkspaceResource: vi.fn(),
  handleK8sError: vi.fn((_error: unknown, action: string) => {
    return new Response(JSON.stringify({ error: `Failed to ${action}` }), { status: 500 });
  }),
  CRD_ARENA_SOURCES: "arenasources",
  createAuditContext: vi.fn(() => ({ workspace: "test-ws" })),
  auditSuccess: vi.fn(),
  auditError: vi.fn(),
  notFoundResponse: vi.fn((message: string) => {
    return new Response(JSON.stringify({ error: message }), { status: 404 });
  }),
}));

vi.mock("fs", () => ({
  existsSync: (...args: unknown[]) => mockExistsSync(...args),
  readFileSync: (...args: unknown[]) => mockReadFileSync(...args),
  writeFileSync: (...args: unknown[]) => mockWriteFileSync(...args),
  readdirSync: (...args: unknown[]) => mockReaddirSync(...args),
  statSync: (...args: unknown[]) => mockStatSync(...args),
  mkdirSync: (...args: unknown[]) => mockMkdirSync(...args),
}));

const mockWorkspace = {
  metadata: { name: "test-ws" },
  spec: { namespace: { name: "test-ns" } },
};

const mockClientOptions = {
  workspace: "test-ws",
  namespace: "test-ns",
  role: "editor",
};

const mockSource = {
  metadata: { name: "test-source" },
  status: { phase: "Ready" },
};

function createMockGetRequest(): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/arena/sources/test-source/versions";
  return new NextRequest(url, { method: "GET" });
}

function createMockPostRequest(body: object): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/arena/sources/test-source/versions";
  return new NextRequest(url, {
    method: "POST",
    body: JSON.stringify(body),
    headers: { "Content-Type": "application/json" },
  });
}

function createMockContext() {
  return {
    params: Promise.resolve({ name: "test-ws", sourceName: "test-source" }),
  };
}

describe("GET /api/workspaces/[name]/arena/sources/[sourceName]/versions", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns versions list for authenticated user with access", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      resource: mockSource as any,
      clientOptions: mockClientOptions as any,
    });

    // Mock filesystem - return true for all paths (base case is always true)
    mockExistsSync.mockReturnValue(true);

    mockReadFileSync.mockReturnValue("abc123def456\n");

    mockReaddirSync.mockImplementation((path: string) => {
      if (path.includes("versions")) {
        return [
          { name: "abc123def456", isDirectory: () => true, isFile: () => false },
          { name: "xyz789abc012", isDirectory: () => true, isFile: () => false },
        ];
      }
      // For version directory contents
      return [
        { name: "file1.yaml", isDirectory: () => false, isFile: () => true },
        { name: "file2.yaml", isDirectory: () => false, isFile: () => true },
      ];
    });

    mockStatSync.mockImplementation((path: string) => {
      if (path.includes("abc123def456")) {
        return {
          isDirectory: () => true,
          birthtime: new Date("2026-01-20T10:00:00Z"),
          size: 512,
        };
      }
      if (path.includes("xyz789abc012")) {
        return {
          isDirectory: () => true,
          birthtime: new Date("2026-01-19T10:00:00Z"),
          size: 256,
        };
      }
      // For individual files
      return {
        isDirectory: () => false,
        isFile: () => true,
        birthtime: new Date("2026-01-20T10:00:00Z"),
        size: 256,
      };
    });

    const { GET } = await import("./route");
    const response = await GET(createMockGetRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();

    expect(body.sourceName).toBe("test-source");
    expect(body.head).toBe("abc123def456");
    expect(body.versions).toBeDefined();
    expect(Array.isArray(body.versions)).toBe(true);
  });

  it("returns 404 when source content directory does not exist", async () => {
    const { getWorkspaceResource, notFoundResponse } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      resource: mockSource as any,
      clientOptions: mockClientOptions as any,
    });

    mockExistsSync.mockReturnValue(false);
    vi.mocked(notFoundResponse).mockReturnValue(
      new Response(JSON.stringify({ error: "Not found" }), { status: 404 }) as any
    );

    const { GET } = await import("./route");
    const response = await GET(createMockGetRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns 404 when source is not ready", async () => {
    const { getWorkspaceResource, notFoundResponse } = await import("@/lib/k8s/workspace-route-helpers");

    const notReadySource = {
      metadata: { name: "test-source" },
      status: { phase: "Pending" },
    };

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      resource: notReadySource as any,
      clientOptions: mockClientOptions as any,
    });

    mockExistsSync.mockReturnValue(false);
    vi.mocked(notFoundResponse).mockReturnValue(
      new Response(JSON.stringify({ error: "Source is not ready" }), { status: 404 }) as any
    );

    const { GET } = await import("./route");
    const response = await GET(createMockGetRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns empty versions when no versions directory exists", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      resource: mockSource as any,
      clientOptions: mockClientOptions as any,
    });

    mockExistsSync.mockImplementation((path: string) => !path.includes("versions"));

    mockReadFileSync.mockReturnValue("abc123\n");

    const { GET } = await import("./route");
    const response = await GET(createMockGetRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.versions).toEqual([]);
  });

  it("handles error when getWorkspaceResource fails", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: false,
      response: new Response(JSON.stringify({ error: "Not found" }), { status: 404 }) as any,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockGetRequest(), createMockContext());

    expect(response.status).toBe(404);
  });
});

describe("POST /api/workspaces/[name]/arena/sources/[sourceName]/versions", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("switches version successfully", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      resource: mockSource as any,
      clientOptions: mockClientOptions as any,
    });

    mockExistsSync.mockReturnValue(true);
    mockReadFileSync.mockReturnValue("old123\n");
    mockWriteFileSync.mockImplementation(() => {});
    mockMkdirSync.mockImplementation(() => undefined);
    mockStatSync.mockReturnValue({ isDirectory: () => true });

    const { POST } = await import("./route");
    const response = await POST(createMockPostRequest({ version: "new456" }), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();

    expect(body.success).toBe(true);
    expect(body.sourceName).toBe("test-source");
    expect(body.previousHead).toBe("old123");
    expect(body.newHead).toBe("new456");

    expect(mockWriteFileSync).toHaveBeenCalled();
  });

  it("returns 400 when version field is missing", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      resource: mockSource as any,
      clientOptions: mockClientOptions as any,
    });

    const { POST } = await import("./route");
    const response = await POST(createMockPostRequest({}), createMockContext());

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.error).toContain("version");
  });

  it("returns 400 when version field is not a string", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      resource: mockSource as any,
      clientOptions: mockClientOptions as any,
    });

    const { POST } = await import("./route");
    const response = await POST(createMockPostRequest({ version: 123 }), createMockContext());

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.error).toContain("version");
  });

  it("returns 404 when source content directory does not exist for POST", async () => {
    const { getWorkspaceResource, notFoundResponse } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      resource: mockSource as any,
      clientOptions: mockClientOptions as any,
    });

    mockExistsSync.mockReturnValue(false);
    vi.mocked(notFoundResponse).mockReturnValue(
      new Response(JSON.stringify({ error: "Not found" }), { status: 404 }) as any
    );

    const { POST } = await import("./route");
    const response = await POST(createMockPostRequest({ version: "abc123" }), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns 404 when target version does not exist", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      resource: mockSource as any,
      clientOptions: mockClientOptions as any,
    });

    mockExistsSync.mockImplementation((path: string) => !path.includes("versions"));

    const { POST } = await import("./route");
    const response = await POST(createMockPostRequest({ version: "nonexistent" }), createMockContext());

    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.error).toContain("Version not found");
  });

  it("handles error when getWorkspaceResource fails", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: false,
      response: new Response(JSON.stringify({ error: "Not found" }), { status: 404 }) as any,
    });

    const { POST } = await import("./route");
    const response = await POST(createMockPostRequest({ version: "abc123" }), createMockContext());

    expect(response.status).toBe(404);
  });

  it("creates .arena directory if it does not exist", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      resource: mockSource as any,
      clientOptions: mockClientOptions as any,
    });

    mockExistsSync.mockImplementation((path: string) => !path.endsWith(".arena"));
    mockReadFileSync.mockReturnValue("old123\n");
    mockWriteFileSync.mockImplementation(() => {});
    mockMkdirSync.mockImplementation(() => undefined);
    mockStatSync.mockReturnValue({ isDirectory: () => true });

    const { POST } = await import("./route");
    await POST(createMockPostRequest({ version: "new456" }), createMockContext());

    expect(mockMkdirSync).toHaveBeenCalled();
  });
});

describe("Helper functions", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.resetAllMocks();
  });

  it("readHeadVersion returns null when HEAD file does not exist", async () => {
    // Mock existsSync to return true for base path but handle HEAD file specially
    mockExistsSync.mockImplementation((path: string) => !path.includes("HEAD"));

    // Import to access the functions through GET/POST calls
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      resource: mockSource as any,
      clientOptions: mockClientOptions as any,
    });

    mockReaddirSync.mockReturnValue([]);

    const { GET } = await import("./route");
    const response = await GET(createMockGetRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.head).toBeNull();
  });

  it("handles fs.readFileSync error gracefully", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      resource: mockSource as any,
      clientOptions: mockClientOptions as any,
    });

    mockExistsSync.mockReturnValue(true);
    mockReadFileSync.mockImplementation(() => {
      throw new Error("Read error");
    });
    mockReaddirSync.mockReturnValue([]);

    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    const { GET } = await import("./route");
    const response = await GET(createMockGetRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.head).toBeNull();

    consoleSpy.mockRestore();
  });

  it("getVersionMetadata handles non-directory entries", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      resource: mockSource as any,
      clientOptions: mockClientOptions as any,
    });

    mockExistsSync.mockReturnValue(true);
    mockReadFileSync.mockReturnValue("abc123\n");
    mockReaddirSync.mockImplementation((path: string) => {
      if (path.includes("versions")) {
        return [
          { name: "abc123", isDirectory: () => true, isFile: () => false },
        ];
      }
      return [];
    });

    mockStatSync.mockImplementation((path: string) => {
      // First call returns non-directory (simulating metadata read)
      if (path.includes("abc123")) {
        return {
          isDirectory: () => false,
          birthtime: new Date("2026-01-20T10:00:00Z"),
          size: 100,
        };
      }
      return {
        isDirectory: () => true,
        birthtime: new Date("2026-01-20T10:00:00Z"),
        size: 100,
      };
    });

    const { GET } = await import("./route");
    const response = await GET(createMockGetRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    // Version with non-directory stat should be filtered out
    expect(body.versions.length).toBe(0);
  });
});

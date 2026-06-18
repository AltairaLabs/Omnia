/**
 * Tests for Arena source versions API route.
 *
 * The route now calls the operator content API (content-api-service /
 * content-tree) instead of node:fs. Mock shapes match the Go content.Listing /
 * content.FileContent json tags (mock-to-contract).
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest, NextResponse } from "next/server";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

type RouteHandler = (
  request: NextRequest,
  context: { params: Promise<{ name: string; sourceName: string }> },
  access: WorkspaceAccess,
  user: User
) => Promise<NextResponse>;

const mockUser: User = {
  id: "testuser",
  provider: "oauth" as const,
  username: "testuser",
  email: "test@example.com",
  groups: [],
  role: "admin" as const,
};

// Mock dependencies before imports
vi.mock("@/lib/auth/workspace-guard", () => ({
  withWorkspaceAccess: vi.fn((_role: string, handler: RouteHandler) => {
    return async (request: NextRequest, context: { params: Promise<{ name: string; sourceName: string }> }) => {
      const mockAccess: WorkspaceAccess = {
        granted: true,
        role: "editor",
        permissions: { read: true, write: true, delete: false, manageMembers: false },
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

const BASE = "arena/test-source";
const HEAD = `${BASE}/.arena/HEAD`;
const VERSIONS = `${BASE}/.arena/versions`;

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

async function mockResourceOk(resource: unknown = mockSource) {
  const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
  vi.mocked(getWorkspaceResource).mockResolvedValue({
    ok: true,
    workspace: mockWorkspace,
    resource,
    clientOptions: mockClientOptions,
  } as any);
}

describe("GET /api/workspaces/[name]/arena/sources/[sourceName]/versions", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns versions list (newest first, with metadata) for a user with access", async () => {
    await mockResourceOk();
    const svc = await import("@/lib/data/content-api-service");
    const tree = await import("@/lib/data/content-tree");

    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === BASE) {
        return { path: relpath, entries: [] };
      }
      if (relpath === HEAD) {
        return { path: relpath, content: "abc123def456\n", encoding: "utf-8", size: 13, modifiedAt: "t" };
      }
      if (relpath === VERSIONS) {
        return {
          path: relpath,
          entries: [
            { name: "abc123def456", type: "directory", size: 0, modifiedAt: "2026-01-20T10:00:00Z" },
            { name: "xyz789abc012", type: "directory", size: 0, modifiedAt: "2026-01-19T10:00:00Z" },
            { name: ".hidden", type: "directory", size: 0, modifiedAt: "2026-01-21T10:00:00Z" },
          ],
        };
      }
      throw new svc.ContentApiError("not found", 404);
    });

    vi.mocked(tree.listContentTree).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath.endsWith("abc123def456")) {
        return [
          { name: "a.yaml", path: `${relpath}/a.yaml`, isDirectory: false, size: 300, modifiedAt: "t" },
          { name: "b.yaml", path: `${relpath}/b.yaml`, isDirectory: false, size: 212, modifiedAt: "t" },
        ];
      }
      return [{ name: "c.yaml", path: `${relpath}/c.yaml`, isDirectory: false, size: 256, modifiedAt: "t" }];
    });

    const { GET } = await import("./route");
    const response = await GET(createMockGetRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();

    expect(body.sourceName).toBe("test-source");
    expect(body.head).toBe("abc123def456");
    expect(body.versions).toHaveLength(2); // hidden dir filtered out
    // Newest first.
    expect(body.versions[0].hash).toBe("abc123def456");
    expect(body.versions[0].fileCount).toBe(2);
    expect(body.versions[0].size).toBe(512);
    expect(body.versions[0].createdAt).toBe("2026-01-20T10:00:00Z");
    expect(body.versions[0].isLatest).toBe(true);
    expect(body.versions[1].hash).toBe("xyz789abc012");
    expect(body.versions[1].isLatest).toBe(false);
  });

  it("tallies nested directories when computing version size and file count", async () => {
    await mockResourceOk();
    const svc = await import("@/lib/data/content-api-service");
    const tree = await import("@/lib/data/content-tree");

    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === BASE) {
        return { path: relpath, entries: [] };
      }
      if (relpath === HEAD) {
        return { path: relpath, content: "abc123\n", encoding: "utf-8", size: 7, modifiedAt: "t" };
      }
      if (relpath === VERSIONS) {
        return {
          path: relpath,
          entries: [{ name: "abc123", type: "directory", size: 0, modifiedAt: "2026-01-20T10:00:00Z" }],
        };
      }
      throw new svc.ContentApiError("not found", 404);
    });

    vi.mocked(tree.listContentTree).mockResolvedValue([
      { name: "top.yaml", path: "x/top.yaml", isDirectory: false, size: 100, modifiedAt: "t" },
      {
        name: "sub",
        path: "x/sub",
        isDirectory: true,
        modifiedAt: "t",
        children: [
          { name: "deep.yaml", path: "x/sub/deep.yaml", isDirectory: false, size: 50, modifiedAt: "t" },
        ],
      },
    ]);

    const { GET } = await import("./route");
    const response = await GET(createMockGetRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.versions[0].fileCount).toBe(2);
    expect(body.versions[0].size).toBe(150);
  });

  it("rethrows non-404 content errors as a 500", async () => {
    await mockResourceOk();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("boom", 500));

    const { GET } = await import("./route");
    const response = await GET(createMockGetRequest(), createMockContext());

    expect(response.status).toBe(500);
  });

  it("returns 404 when source content directory does not exist", async () => {
    await mockResourceOk();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { GET } = await import("./route");
    const response = await GET(createMockGetRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns 404 when source is not ready", async () => {
    await mockResourceOk({ metadata: { name: "test-source" }, status: { phase: "Pending" } });
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { GET } = await import("./route");
    const response = await GET(createMockGetRequest(), createMockContext());

    expect(response.status).toBe(404);
    expect((await response.json()).error).toContain("not ready");
  });

  it("returns empty versions when no versions directory exists", async () => {
    await mockResourceOk();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === BASE) {
        return { path: relpath, entries: [] };
      }
      if (relpath === HEAD) {
        return { path: relpath, content: "abc123\n", encoding: "utf-8", size: 7, modifiedAt: "t" };
      }
      // versions dir missing
      throw new svc.ContentApiError("not found", 404);
    });

    const { GET } = await import("./route");
    const response = await GET(createMockGetRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.versions).toEqual([]);
    expect(body.head).toBe("abc123");
  });

  it("returns null head when HEAD file does not exist", async () => {
    await mockResourceOk();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === BASE) {
        return { path: relpath, entries: [] };
      }
      // HEAD + versions missing
      throw new svc.ContentApiError("not found", 404);
    });

    const { GET } = await import("./route");
    const response = await GET(createMockGetRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.head).toBeNull();
    expect(body.versions).toEqual([]);
  });

  it("handles error when getWorkspaceResource fails", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: false,
      response: new Response(JSON.stringify({ error: "Not found" }), { status: 404 }),
    } as any);

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
    await mockResourceOk();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === HEAD) {
        return { path: relpath, content: "old123\n", encoding: "utf-8", size: 7, modifiedAt: "t" };
      }
      // base path + target version exist
      return { path: relpath, entries: [] };
    });
    vi.mocked(svc.writeContentFile).mockResolvedValue({ path: HEAD, size: 7, modifiedAt: "t" });
    vi.mocked(svc.makeContentDir).mockResolvedValue({ path: `${BASE}/.arena`, size: 0, modifiedAt: "t", directory: true });

    const { POST } = await import("./route");
    const response = await POST(createMockPostRequest({ version: "new456" }), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();

    expect(body.success).toBe(true);
    expect(body.sourceName).toBe("test-source");
    expect(body.previousHead).toBe("old123");
    expect(body.newHead).toBe("new456");

    expect(vi.mocked(svc.writeContentFile)).toHaveBeenCalledWith("test-ws", mockUser, HEAD, "new456\n");
    expect(vi.mocked(svc.makeContentDir)).toHaveBeenCalledWith("test-ws", mockUser, `${BASE}/.arena`);
  });

  it("returns 400 when version field is missing", async () => {
    await mockResourceOk();

    const { POST } = await import("./route");
    const response = await POST(createMockPostRequest({}), createMockContext());

    expect(response.status).toBe(400);
    expect((await response.json()).error).toContain("version");
  });

  it("returns 400 when version field is not a string", async () => {
    await mockResourceOk();

    const { POST } = await import("./route");
    const response = await POST(createMockPostRequest({ version: 123 }), createMockContext());

    expect(response.status).toBe(400);
    expect((await response.json()).error).toContain("version");
  });

  it("returns 404 when source content directory does not exist for POST", async () => {
    await mockResourceOk();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { POST } = await import("./route");
    const response = await POST(createMockPostRequest({ version: "abc123" }), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns 404 when target version does not exist", async () => {
    await mockResourceOk();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === BASE) {
        return { path: relpath, entries: [] };
      }
      // target version dir missing
      throw new svc.ContentApiError("not found", 404);
    });

    const { POST } = await import("./route");
    const response = await POST(createMockPostRequest({ version: "nonexistent" }), createMockContext());

    expect(response.status).toBe(404);
    expect((await response.json()).error).toContain("Version not found");
  });

  it("handles error when getWorkspaceResource fails", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: false,
      response: new Response(JSON.stringify({ error: "Not found" }), { status: 404 }),
    } as any);

    const { POST } = await import("./route");
    const response = await POST(createMockPostRequest({ version: "abc123" }), createMockContext());

    expect(response.status).toBe(404);
  });
});

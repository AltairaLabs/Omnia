/**
 * Tests for arena config versions API route.
 *
 * The route now calls the operator content API via content-api-service /
 * content-tree; these mock those services (mock-to-contract: shapes match the
 * Go content.Listing / content.FileContent json tags). The on-disk namespace
 * lookup (getWorkspace) is gone — the operator scopes paths itself.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/data/content-api-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/data/content-api-service")>();
  return { ...actual, getContent: vi.fn() };
});

vi.mock("@/lib/data/content-tree", () => ({ listContentTree: vi.fn() }));

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

function createMockRequest(): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/content/arena/test-config/versions";
  return new NextRequest(url, { method: "GET" });
}

function createMockContext() {
  return {
    params: Promise.resolve({ name: "test-ws", config: "test-config" }),
  };
}

async function grantAccess() {
  const { getUser } = await import("@/lib/auth");
  const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
  vi.mocked(getUser).mockResolvedValue(mockUser);
  vi.mocked(checkWorkspaceAccess).mockResolvedValue({
    granted: true,
    role: "viewer",
    permissions: viewerPermissions,
  });
}

describe("GET /api/workspaces/[name]/content/arena/[config]/versions", () => {
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

  it("passes through 404 when the arena config does not exist", async () => {
    await grantAccess();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.error).toBe("Not Found");
  });

  it("returns versions list for valid arena config", async () => {
    await grantAccess();
    const svc = await import("@/lib/data/content-api-service");
    const { listContentTree } = await import("@/lib/data/content-tree");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === "arena/test-config") {
        return { path: relpath, entries: [] };
      }
      if (relpath.endsWith("/.arena/HEAD")) {
        return { path: relpath, content: "abc123", encoding: "utf-8", size: 6, modifiedAt: "t" };
      }
      if (relpath.endsWith("/.arena/versions")) {
        return {
          path: relpath,
          entries: [
            { name: "abc123", type: "directory", size: 0, modifiedAt: "2025-01-02T00:00:00Z" },
            { name: "def456", type: "directory", size: 0, modifiedAt: "2025-01-01T00:00:00Z" },
          ],
        };
      }
      return { path: relpath, entries: [] };
    });
    // Each version dir sums to 1024 bytes (one file).
    vi.mocked(listContentTree).mockResolvedValue([
      { name: "config.yaml", path: "config.yaml", isDirectory: false, size: 1024, modifiedAt: "t" },
    ]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.configName).toBe("test-config");
    expect(body.head).toBe("abc123");
    expect(body.versions.length).toBe(2);
    // Newest first.
    expect(body.versions[0].hash).toBe("abc123");
    expect(body.versions[0].size).toBe(1024);
  });

  it("returns empty versions when there is no HEAD and no versions directory", async () => {
    await grantAccess();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath === "arena/test-config") {
        return { path: relpath, entries: [] };
      }
      // HEAD and versions dir both 404.
      throw new svc.ContentApiError("not found", 404);
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.head).toBe(null);
    expect(body.versions).toEqual([]);
  });
});

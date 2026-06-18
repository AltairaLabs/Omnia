/**
 * Tests for workspace content filesystem API route.
 *
 * The route now calls the operator content API via content-api-service; these
 * mock that service (mock-to-contract: shapes match the Go content.Listing /
 * content.FileContent json tags). Path-confinement, max-size and text/binary
 * encoding are operator-side, surfaced here as pass-through statuses.
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
  return { params: Promise.resolve({ name: "test-ws", path: pathSegments }) };
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

  it("passes through 404 when the path does not exist", async () => {
    await grantAccess();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
    expect((await response.json()).error).toBe("Not Found");
  });

  it("passes through 400 for a path the operator rejects (traversal)", async () => {
    await grantAccess();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("invalid path", 400));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext(["..", "..", "etc", "passwd"]));

    expect(response.status).toBe(400);
  });

  it("returns a directory listing for a directory path", async () => {
    await grantAccess();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockResolvedValue({
      path: "arena/test-config",
      entries: [
        { name: "config.yaml", type: "file", size: 20, modifiedAt: "2025-01-01T00:00:00Z" },
        { name: "tools", type: "directory", size: 0, modifiedAt: "2025-01-01T00:00:00Z" },
        { name: "prompts", type: "directory", size: 0, modifiedAt: "2025-01-01T00:00:00Z" },
        { name: "README.md", type: "file", size: 5, modifiedAt: "2025-01-01T00:00:00Z" },
      ],
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.path).toBe("arena/test-config");
    expect(body.directories.map((d: { name: string }) => d.name)).toEqual(["prompts", "tools"]);
    expect(body.files.map((f: { name: string }) => f.name)).toEqual(["config.yaml", "README.md"]);
  });

  it("returns 400 when requesting file=true on a directory", async () => {
    await grantAccess();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockResolvedValue({ path: "arena/test-config", entries: [] });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest({ file: "true" }), createMockContext());

    expect(response.status).toBe(400);
    expect((await response.json()).message).toContain("Cannot return file content for a directory");
  });

  it("returns file info for a file path without file=true", async () => {
    await grantAccess();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockResolvedValue({
      path: "arena/test-config/config.yaml",
      content: "ignored",
      encoding: "utf-8",
      size: 1024,
      modifiedAt: "2025-01-01T00:00:00Z",
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext(["arena", "test-config", "config.yaml"]));

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.name).toBe("config.yaml");
    expect(body.type).toBe("file");
    expect(body.size).toBe(1024);
    expect(body.content).toBeUndefined();
  });

  it("returns file content for a file path with file=true", async () => {
    await grantAccess();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockResolvedValue({
      path: "arena/test-config/config.yaml",
      content: "apiVersion: v1\nkind: Arena",
      encoding: "utf-8",
      size: 27,
      modifiedAt: "2025-01-01T00:00:00Z",
    });

    const { GET } = await import("./route");
    const response = await GET(
      createMockRequest({ file: "true" }),
      createMockContext(["arena", "test-config", "config.yaml"]),
    );

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.content).toBe("apiVersion: v1\nkind: Arena");
    expect(body.encoding).toBe("utf-8");
  });

  it("passes through the operator's base64 encoding for binary files", async () => {
    await grantAccess();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockResolvedValue({
      path: "arena/test-config/image.png",
      content: "AAAA",
      encoding: "base64",
      size: 3,
      modifiedAt: "2025-01-01T00:00:00Z",
    });

    const { GET } = await import("./route");
    const response = await GET(
      createMockRequest({ file: "true" }),
      createMockContext(["arena", "test-config", "image.png"]),
    );

    expect(response.status).toBe(200);
    expect((await response.json()).encoding).toBe("base64");
  });

  it("passes through 413 for files the operator rejects as too large", async () => {
    await grantAccess();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("file too large", 413));

    const { GET } = await import("./route");
    const response = await GET(
      createMockRequest({ file: "true" }),
      createMockContext(["arena", "test-config", "large.bin"]),
    );

    expect(response.status).toBe(413);
  });

  it("resolves ?version=latest via the arena HEAD file", async () => {
    await grantAccess();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath.endsWith("/.arena/HEAD")) {
        return { path: relpath, content: "abc123", encoding: "utf-8", size: 6, modifiedAt: "t" };
      }
      return { path: relpath, entries: [] };
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest({ version: "latest" }), createMockContext(["arena", "test-config"]));

    expect(response.status).toBe(200);
    // The listing was fetched from the resolved version directory.
    expect(vi.mocked(svc.getContent)).toHaveBeenCalledWith(
      "test-ws",
      mockUser,
      "arena/test-config/.arena/versions/abc123",
    );
  });
});

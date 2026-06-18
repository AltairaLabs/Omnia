/**
 * Tests for Arena source single-file API route.
 *
 * GET /api/workspaces/:name/arena/sources/:sourceName/file?path=<relativePath>
 *
 * The route now calls the operator content API (content-api-service) instead of
 * node:fs. Path confinement, max-size and text/binary encoding are operator-side,
 * surfaced here as pass-through statuses. Mock shapes match the Go
 * content.Listing / content.FileContent json tags (mock-to-contract).
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

import type { ContentNode } from "@/lib/data/content-api-service";

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/k8s/crd-operations", () => ({
  getCrd: vi.fn(),
  extractK8sErrorMessage: vi.fn((error: unknown) =>
    error instanceof Error ? error.message : String(error)
  ),
  isForbiddenError: vi.fn(() => false),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/workspace-route-helpers")>();
  return {
    ...actual,
    validateWorkspace: vi.fn(),
    getWorkspaceResource: vi.fn(),
    createAuditContext: vi.fn(() => ({
      workspace: "test-ws",
      namespace: "test-ns",
      user: { username: "testuser" },
      role: "viewer",
      resourceType: "ArenaSource",
    })),
    auditSuccess: vi.fn(),
    auditError: vi.fn(),
  };
});

vi.mock("@/lib/audit", () => ({
  logError: vi.fn(),
  logCrdSuccess: vi.fn(),
  logCrdDenied: vi.fn(),
  logCrdError: vi.fn(),
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

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

const mockSource = {
  metadata: { name: "test-source", namespace: "test-ns" },
  spec: { type: "git" },
  status: { phase: "Ready" },
};

const T = "2025-01-01T00:00:00Z";

function createMockRequest(queryParams?: Record<string, string>): NextRequest {
  const url = new URL("http://localhost:3000/api/workspaces/test-ws/arena/sources/test-source/file");
  if (queryParams) {
    for (const [key, value] of Object.entries(queryParams)) {
      url.searchParams.set(key, value);
    }
  }
  return new NextRequest(url.toString(), { method: "GET" });
}

function createMockContext() {
  return {
    params: Promise.resolve({ name: "test-ws", sourceName: "test-source" }),
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

async function mockResourceOk() {
  const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
  vi.mocked(getWorkspaceResource).mockResolvedValue({
    ok: true,
    resource: mockSource,
    workspace: mockWorkspace,
    clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
  } as any);
}

/** A getContent mock that resolves HEAD (legacy: missing) + a file at the given path. */
async function mockLegacyFile(fileRelpath: string, file: ContentNode) {
  const svc = await import("@/lib/data/content-api-service");
  vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
    if (relpath.endsWith("/.arena/HEAD")) {
      throw new svc.ContentApiError("not found", 404);
    }
    if (relpath === "arena/test-source") {
      // base path listing with legacy content
      return { path: relpath, entries: [{ name: "visible.yaml", type: "file", size: 1, modifiedAt: T }] };
    }
    if (relpath === fileRelpath) {
      return file;
    }
    throw new svc.ContentApiError("not found", 404);
  });
}

describe("GET /api/workspaces/[name]/arena/sources/[sourceName]/file", () => {
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
    const response = await GET(createMockRequest({ path: "config.yaml" }), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 400 when the path query parameter is missing", async () => {
    await grantAccess();

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(400);
    expect((await response.json()).error).toContain("path");
  });

  it("returns 404 when the source is not found", async () => {
    await grantAccess();
    const { getWorkspaceResource, notFoundResponse } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: false,
      response: notFoundResponse("Arena source not found"),
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest({ path: "config.yaml" }), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns 404 when no source content is available", async () => {
    await grantAccess();
    await mockResourceOk();
    const svc = await import("@/lib/data/content-api-service");
    // No HEAD, and base path is missing.
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("not found", 404));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest({ path: "config.yaml" }), createMockContext());

    expect(response.status).toBe(404);
    expect((await response.json()).error).toContain("not available");
  });

  it("returns file content for a legacy-layout file", async () => {
    await grantAccess();
    await mockResourceOk();
    await mockLegacyFile("arena/test-source/config.yaml", {
      path: "arena/test-source/config.yaml",
      content: "apiVersion: v1",
      encoding: "utf-8",
      size: 14,
      modifiedAt: T,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest({ path: "config.yaml" }), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.path).toBe("config.yaml");
    expect(body.content).toBe("apiVersion: v1");
    expect(body.size).toBe(14);
  });

  it("resolves the HEAD version directory before reading the file", async () => {
    await grantAccess();
    await mockResourceOk();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath.endsWith("/.arena/HEAD")) {
        return { path: relpath, content: "abc123\n", encoding: "utf-8", size: 7, modifiedAt: T };
      }
      return { path: relpath, content: "hello", encoding: "utf-8", size: 5, modifiedAt: T };
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest({ path: "sub/file.yaml" }), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.path).toBe("sub/file.yaml");
    expect(body.content).toBe("hello");
    // File read from the resolved version directory.
    expect(vi.mocked(svc.getContent)).toHaveBeenCalledWith(
      "test-ws",
      mockUser,
      "arena/test-source/.arena/versions/abc123/sub/file.yaml",
    );
  });

  it("returns 400 when the path is a directory", async () => {
    await grantAccess();
    await mockResourceOk();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath.endsWith("/.arena/HEAD")) {
        throw new svc.ContentApiError("not found", 404);
      }
      if (relpath === "arena/test-source") {
        return { path: relpath, entries: [{ name: "sub", type: "directory", size: 0, modifiedAt: T }] };
      }
      return { path: relpath, entries: [] };
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest({ path: "sub" }), createMockContext());

    expect(response.status).toBe(400);
    expect((await response.json()).error).toContain("directory");
  });

  it("passes through the operator's base64 encoding without altering size", async () => {
    await grantAccess();
    await mockResourceOk();
    await mockLegacyFile("arena/test-source/image.png", {
      path: "arena/test-source/image.png",
      content: "AAAA",
      encoding: "base64",
      size: 3,
      modifiedAt: T,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest({ path: "image.png" }), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.content).toBe("AAAA");
    expect(body.size).toBe(3);
  });

  it("passes through a 400 the operator raises for a traversal path", async () => {
    await grantAccess();
    await mockResourceOk();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath.endsWith("/.arena/HEAD")) {
        throw new svc.ContentApiError("not found", 404);
      }
      if (relpath === "arena/test-source") {
        return { path: relpath, entries: [{ name: "visible.yaml", type: "file", size: 1, modifiedAt: T }] };
      }
      throw new svc.ContentApiError("invalid path", 400);
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest({ path: "../../etc/passwd" }), createMockContext());

    expect(response.status).toBe(400);
  });

  it("passes through a 413 the operator raises for an oversized file", async () => {
    await grantAccess();
    await mockResourceOk();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockImplementation(async (_ws, _user, relpath = "") => {
      if (relpath.endsWith("/.arena/HEAD")) {
        throw new svc.ContentApiError("not found", 404);
      }
      if (relpath === "arena/test-source") {
        return { path: relpath, entries: [{ name: "visible.yaml", type: "file", size: 1, modifiedAt: T }] };
      }
      throw new svc.ContentApiError("file too large", 413);
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest({ path: "large.bin" }), createMockContext());

    expect(response.status).toBe(413);
  });

  it("handles K8s errors gracefully", async () => {
    await grantAccess();
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    vi.mocked(getWorkspaceResource).mockRejectedValue(new Error("K8s connection failed"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest({ path: "config.yaml" }), createMockContext());

    expect(response.status).toBe(500);
  });
});

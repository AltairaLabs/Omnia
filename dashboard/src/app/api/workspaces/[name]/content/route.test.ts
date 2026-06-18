/**
 * Tests for workspace content root listing API route.
 *
 * The route now calls the operator content API via content-api-service instead
 * of reading the NFS mount, so these mock that service (mock-to-contract: the
 * mocked shapes match the Go content.Listing json tags).
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

function createMockRequest(): NextRequest {
  return new NextRequest("http://localhost:3000/api/workspaces/test-ws/content", { method: "GET" });
}

function createMockContext() {
  return { params: Promise.resolve({ name: "test-ws" }) };
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

describe("GET /api/workspaces/[name]/content", () => {
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

  it("splits the operator listing into files and directories", async () => {
    await grantAccess();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockResolvedValue({
      path: "",
      entries: [
        { name: "README.md", type: "file", size: 10, modifiedAt: "2025-01-01T00:00:00Z" },
        { name: "skills", type: "directory", size: 0, modifiedAt: "2025-01-01T00:00:00Z" },
        { name: "arena", type: "directory", size: 0, modifiedAt: "2025-01-01T00:00:00Z" },
        { name: "AGENTS.md", type: "file", size: 5, modifiedAt: "2025-01-01T00:00:00Z" },
      ],
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.path).toBe("/");
    // Sorted alphabetically within each group.
    expect(body.directories.map((d: { name: string }) => d.name)).toEqual(["arena", "skills"]);
    expect(body.files.map((f: { name: string }) => f.name)).toEqual(["AGENTS.md", "README.md"]);
    // Called with the empty root path for the authenticated user.
    expect(vi.mocked(svc.getContent)).toHaveBeenCalledWith("test-ws", mockUser, "");
  });

  it("returns an empty listing for an empty workspace", async () => {
    await grantAccess();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockResolvedValue({ path: "", entries: [] });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.files).toEqual([]);
    expect(body.directories).toEqual([]);
  });

  it("passes through the operator status (404) when the workspace is unknown", async () => {
    await grantAccess();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new svc.ContentApiError("workspace not found", 404));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.error).toBe("Not Found");
  });

  it("returns 500 on an unexpected error", async () => {
    await grantAccess();
    const svc = await import("@/lib/data/content-api-service");
    vi.mocked(svc.getContent).mockRejectedValue(new Error("boom"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(500);
  });
});

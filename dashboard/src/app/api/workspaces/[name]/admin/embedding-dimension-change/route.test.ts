/**
 * Tests for the embedding-dimension-change admin proxy route (#1309).
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({ getUser: vi.fn() }));
vi.mock("@/lib/auth/workspace-authz", () => ({ checkWorkspaceAccess: vi.fn() }));
vi.mock("@/lib/k8s/workspace-route-helpers", () => ({ getWorkspace: vi.fn() }));
vi.mock("@/lib/k8s/service-url-resolver", () => ({ resolveServiceURLs: vi.fn() }));

const mockFetch = vi.fn();
global.fetch = mockFetch;

const mockUser = {
  id: "owner-id",
  provider: "oauth" as const,
  username: "owner",
  email: "owner@example.com",
  groups: ["users"],
  role: "viewer" as const,
};
const ownerPermissions = { read: true, write: true, delete: true, manageMembers: true };
const noPermissions = { read: false, write: false, delete: false, manageMembers: false };
const mockWorkspace = { metadata: { uid: "ws-uid-123", name: "test-ws" }, spec: {} };
const serviceURLs = {
  sessionURL: "https://session-api:8080",
  memoryURL: "https://memory-api:8080",
  namespace: "omnia-test", privacyURL: ""
};

function ctx() {
  return { params: Promise.resolve({ name: "test-ws" }) };
}

function postReq(body: string): NextRequest {
  return new NextRequest(
    "https://localhost:3000/api/workspaces/test-ws/admin/embedding-dimension-change",
    { method: "POST", body }
  );
}

function mockFetchJson(data: unknown, status = 200) {
  mockFetch.mockResolvedValueOnce({
    ok: status >= 200 && status < 300,
    status,
    text: () => Promise.resolve(JSON.stringify(data)),
  });
}

describe("POST /api/workspaces/[name]/admin/embedding-dimension-change", () => {
  beforeEach(() => vi.resetModules());
  afterEach(() => vi.resetAllMocks());

  async function setupOwner() {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");
    vi.mocked(resolveServiceURLs).mockResolvedValue(serviceURLs);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "owner",
      permissions: ownerPermissions,
    });
    vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);
  }

  it("proxies the consent POST to memory-api for an owner", async () => {
    await setupOwner();
    mockFetchJson({ status: "consent recorded", target_dim: 768 });

    const { POST } = await import("./route");
    const res = await POST(postReq(JSON.stringify({ target_dim: 768 })), ctx());

    expect(res.status).toBe(200);
    const [fetchUrl, fetchOpts] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(fetchUrl).toContain("https://memory-api:8080/admin/embedding-dimension-change");
    expect(fetchOpts.method).toBe("POST");
    expect(fetchOpts.body).toBe(JSON.stringify({ target_dim: 768 }));
  });

  it("forwards a backend 400 (bad dimension)", async () => {
    await setupOwner();
    mockFetchJson({ error: "target_dim must be between 1 and 2000" }, 400);

    const { POST } = await import("./route");
    const res = await POST(postReq(JSON.stringify({ target_dim: 5000 })), ctx());

    expect(res.status).toBe(400);
  });

  it("returns 403 for a non-owner", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: false,
      role: "editor",
      permissions: noPermissions,
    });

    const { POST } = await import("./route");
    const res = await POST(postReq(JSON.stringify({ target_dim: 768 })), ctx());

    expect(res.status).toBe(403);
    expect(mockFetch).not.toHaveBeenCalled();
  });
});

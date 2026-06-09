import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";
import { pseudonymizeId } from "@/lib/identity";

// --- module mocks (hoisted before any dynamic imports) ---

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", () => ({
  getWorkspace: vi.fn(),
}));

vi.mock("@/lib/k8s/service-url-resolver", () => ({
  resolveServiceURLs: vi.fn(),
}));

const mockFetch = vi.fn();
global.fetch = mockFetch;

const mockUser = {
  id: "testuser-id",
  provider: "oauth" as const,
  username: "testuser",
  email: "test@example.com",
  groups: ["users"],
  role: "viewer" as const,
};

const anonUser = {
  id: "anonymous",
  provider: "anonymous" as const,
  username: "anonymous",
  groups: [],
  role: "viewer" as const,
};

const grantedViewer = {
  granted: true,
  role: "viewer" as const,
  permissions: { read: true, write: false, delete: false, manageMembers: false },
};

const mockWorkspace = { metadata: { uid: "workspace-uid-123", name: "test-ws" }, spec: {} };
const memoryURLs = { sessionURL: "https://session-api:8080", memoryURL: "https://memory-api:8080", namespace: "omnia-test" };

function ctx(memoryId: string) {
  return { params: Promise.resolve({ name: "test-ws", memoryId }) };
}

function deleteReq(query = ""): NextRequest {
  const suffix = query ? "?" + query : "";
  return new NextRequest(`https://localhost:3000/api/workspaces/test-ws/memory/mem-123${suffix}`, { method: "DELETE" });
}

async function wireMocks(user: typeof mockUser | typeof anonUser, access = grantedViewer) {
  const { getUser } = await import("@/lib/auth");
  const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
  const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
  const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");
  vi.mocked(getUser).mockResolvedValue(user as never);
  vi.mocked(checkWorkspaceAccess).mockResolvedValue(access as never);
  vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);
  vi.mocked(resolveServiceURLs).mockResolvedValue(memoryURLs as never);
}

describe("DELETE /memory/[memoryId]", () => {
  beforeEach(() => vi.resetModules());
  afterEach(() => {
    vi.resetAllMocks();
    mockFetch.mockReset();
  });

  it("scopes the delete to the authenticated session user (#1268)", async () => {
    await wireMocks(mockUser);
    mockFetch.mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve({}) });

    const { DELETE } = await import("./route");
    // Even if the client tries to spoof another user's id, the scope is the session user.
    const response = await DELETE(deleteReq("userId=victim-id"), ctx("mem-123"));

    expect(response.status).toBe(200);
    const url = mockFetch.mock.calls[0][0] as string;
    expect(url).toContain("https://memory-api:8080/api/v1/memories/mem-123");
    expect(url).toContain("workspace=workspace-uid-123");
    expect(url).toContain(`user_id=${pseudonymizeId(mockUser.id)}`);
    expect(url).not.toContain(pseudonymizeId("victim-id"));
  });

  it("scopes anonymous deletes to the device pseudonym", async () => {
    await wireMocks(anonUser);
    mockFetch.mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve({}) });

    const { DELETE } = await import("./route");
    const response = await DELETE(deleteReq("userId=device-xyz"), ctx("mem-123"));

    expect(response.status).toBe(200);
    const url = mockFetch.mock.calls[0][0] as string;
    expect(url).toContain(`user_id=${pseudonymizeId("device-xyz")}`);
  });

  it("denies access when the workspace guard rejects", async () => {
    await wireMocks(mockUser, { granted: false, role: null, permissions: { read: false, write: false, delete: false, manageMembers: false } } as never);

    const { DELETE } = await import("./route");
    const response = await DELETE(deleteReq(), ctx("mem-123"));

    expect(response.status).toBe(403);
    expect(mockFetch).not.toHaveBeenCalled();
  });
});

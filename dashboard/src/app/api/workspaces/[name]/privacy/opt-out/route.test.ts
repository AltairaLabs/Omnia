/**
 * Tests for opt-out proxy route.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";
import { pseudonymizeId } from "@/lib/identity";

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/k8s/service-url-resolver", () => ({
  resolveServiceURLs: vi.fn(),
}));

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

const viewerPermissions = { read: true, write: false, delete: false, manageMembers: false };
const noPermissions = { read: false, write: false, delete: false, manageMembers: false };

const mockFetch = vi.fn();
global.fetch = mockFetch;

const PRIVACY_API = "https://privacy-api:8080";

/** Mock ServiceURLs with privacyURL set to the test privacy-api host. */
const resolvedURLs = {
  sessionURL: "https://session-api:8080",
  memoryURL: "https://memory-api:8080",
  privacyURL: PRIVACY_API,
  namespace: "omnia-test",
};

function createGetRequest(query = "?userId=user-123"): NextRequest {
  return new NextRequest(
    `http://localhost:3000/api/workspaces/test-ws/privacy/opt-out${query}`,
    { method: "GET" }
  );
}

function createPostRequest(body: object, query = "?userId=user-123"): NextRequest {
  return new NextRequest(
    `http://localhost:3000/api/workspaces/test-ws/privacy/opt-out${query}`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }
  );
}

function createDeleteRequest(body: object, query = "?userId=user-123"): NextRequest {
  return new NextRequest(
    `http://localhost:3000/api/workspaces/test-ws/privacy/opt-out${query}`,
    {
      method: "DELETE",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }
  );
}

function createMockContext() {
  return { params: Promise.resolve({ name: "test-ws" }) };
}

// privacy-api Preferences struct (camelCase json tags):
// optOutAll, optOutWorkspaces, optOutAgents, consentGrants, userId, createdAt, updatedAt
const mockPreferences = {
  userId: pseudonymizeId(mockUser.id),
  optOutAll: false,
  optOutWorkspaces: ["ws-a"],
  optOutAgents: [],
  consentGrants: ["analytics"],
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-06-01T00:00:00Z",
};

describe("GET /api/workspaces/[name]/privacy/opt-out", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("proxies GET to privacy-api preferences endpoint with hashed userId", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(resolvedURLs);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      text: () => Promise.resolve(JSON.stringify(mockPreferences)),
    });

    const { GET } = await import("./route");
    const response = await GET(createGetRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    // Response shape matches Preferences struct
    expect(body.optOutAll).toBe(false);
    expect(body.optOutWorkspaces).toEqual(["ws-a"]);
    expect(body.consentGrants).toEqual(["analytics"]);

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("privacy-api");
    expect(fetchUrl).not.toContain("session-api");
    expect(fetchUrl).toContain(`/api/v1/privacy/preferences/${pseudonymizeId(mockUser.id)}`);
    // Must not contain the /consent suffix — this is the full Preferences path
    expect(fetchUrl).not.toContain("/consent");
  });

  it("ignores a client-supplied userId and scopes to authenticated user (#1263)", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(resolvedURLs);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      text: () => Promise.resolve(JSON.stringify(mockPreferences)),
    });

    const { GET } = await import("./route");
    const response = await GET(createGetRequest("?userId=victim-id"), createMockContext());

    expect(response.status).toBe(200);
    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain(pseudonymizeId(mockUser.id));
    expect(fetchUrl).not.toContain(pseudonymizeId("victim-id"));
  });

  it("returns 400 when an anonymous user has no device id", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(resolvedURLs);
    vi.mocked(getUser).mockResolvedValue(anonUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const { GET } = await import("./route");
    const response = await GET(createGetRequest(""), createMockContext());

    expect(response.status).toBe(400);
  });

  it("returns 503 when service URLs are not resolvable", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(null);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const { GET } = await import("./route");
    const response = await GET(createGetRequest(), createMockContext());

    expect(response.status).toBe(503);
  });

  it("returns 403 when user lacks workspace access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: false,
      role: null,
      permissions: noPermissions,
    });

    const { GET } = await import("./route");
    const response = await GET(createGetRequest(), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 502 on fetch error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(resolvedURLs);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    mockFetch.mockRejectedValueOnce(new Error("Connection refused"));

    const { GET } = await import("./route");
    const response = await GET(createGetRequest(), createMockContext());

    expect(response.status).toBe(502);
  });

  it("forwards backend status code on non-OK response", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(resolvedURLs);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
      text: () => Promise.resolve(JSON.stringify({ error: "not found" })),
    });

    const { GET } = await import("./route");
    const response = await GET(createGetRequest(), createMockContext());

    expect(response.status).toBe(404);
  });
});

describe("POST /api/workspaces/[name]/privacy/opt-out", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("proxies POST to privacy-api opt-out with { userId: hashedId, scope, target }", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(resolvedURLs);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    // Response shape matches Preferences struct (post returns updated prefs)
    const updatedPrefs = { ...mockPreferences, optOutAll: true };
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      text: () => Promise.resolve(JSON.stringify(updatedPrefs)),
    });

    const { POST } = await import("./route");
    const response = await POST(
      createPostRequest({ scope: "all", target: "" }),
      createMockContext()
    );

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.optOutAll).toBe(true);

    const [fetchUrl, fetchOpts] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(fetchUrl).toContain("privacy-api");
    expect(fetchUrl).not.toContain("session-api");
    expect(fetchUrl).toContain("/api/v1/privacy/opt-out");
    expect(fetchOpts.method).toBe("POST");

    // The outgoing body must use the server-resolved hashed ID, not the
    // client-supplied user-123.
    const sentBody = JSON.parse(fetchOpts.body as string);
    expect(sentBody.userId).toBe(pseudonymizeId(mockUser.id));
    expect(sentBody.userId).not.toBe(pseudonymizeId("user-123"));
    expect(sentBody.scope).toBe("all");
  });

  it("ignores a client-supplied userId in the request body (security: #1263)", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(resolvedURLs);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      text: () => Promise.resolve(JSON.stringify(mockPreferences)),
    });

    const { POST } = await import("./route");
    // Attacker passes victim's userId in query string and body
    const response = await POST(
      createPostRequest({ userId: "victim-id", scope: "all", target: "" }, "?userId=victim-id"),
      createMockContext()
    );

    expect(response.status).toBe(200);
    const [, fetchOpts] = mockFetch.mock.calls[0] as [string, RequestInit];
    const sentBody = JSON.parse(fetchOpts.body as string);
    expect(sentBody.userId).toBe(pseudonymizeId(mockUser.id));
    expect(sentBody.userId).not.toBe(pseudonymizeId("victim-id"));
  });

  it("returns 400 when an anonymous user has no device id", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(resolvedURLs);
    vi.mocked(getUser).mockResolvedValue(anonUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const { POST } = await import("./route");
    const response = await POST(createPostRequest({}, ""), createMockContext());

    expect(response.status).toBe(400);
  });

  it("returns 503 when service URLs are not resolvable", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(null);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const { POST } = await import("./route");
    const response = await POST(createPostRequest({ scope: "all" }), createMockContext());

    expect(response.status).toBe(503);
  });

  it("returns 403 when user lacks workspace access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: false,
      role: null,
      permissions: noPermissions,
    });

    const { POST } = await import("./route");
    const response = await POST(createPostRequest({ scope: "all" }), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 502 on fetch error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(resolvedURLs);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    mockFetch.mockRejectedValueOnce(new Error("Connection refused"));

    const { POST } = await import("./route");
    const response = await POST(createPostRequest({ scope: "all" }), createMockContext());

    expect(response.status).toBe(502);
  });
});

describe("DELETE /api/workspaces/[name]/privacy/opt-out", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("proxies DELETE to privacy-api opt-out with { userId: hashedId, scope, target }", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(resolvedURLs);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    // Response after removing opt-out — Preferences with reduced optOutWorkspaces
    const updatedPrefs = { ...mockPreferences, optOutWorkspaces: [] };
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      text: () => Promise.resolve(JSON.stringify(updatedPrefs)),
    });

    const { DELETE } = await import("./route");
    const response = await DELETE(
      createDeleteRequest({ scope: "workspace", target: "ws-a" }),
      createMockContext()
    );

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.optOutWorkspaces).toEqual([]);

    const [fetchUrl, fetchOpts] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(fetchUrl).toContain("privacy-api");
    expect(fetchUrl).not.toContain("session-api");
    expect(fetchUrl).toContain("/api/v1/privacy/opt-out");
    expect(fetchOpts.method).toBe("DELETE");

    const sentBody = JSON.parse(fetchOpts.body as string);
    expect(sentBody.userId).toBe(pseudonymizeId(mockUser.id));
    expect(sentBody.scope).toBe("workspace");
    expect(sentBody.target).toBe("ws-a");
  });

  it("ignores a client-supplied userId when removing opt-out (security: #1263)", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(resolvedURLs);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      text: () => Promise.resolve(JSON.stringify(mockPreferences)),
    });

    const { DELETE } = await import("./route");
    const response = await DELETE(
      createDeleteRequest({ userId: "victim-id", scope: "all", target: "" }, "?userId=victim-id"),
      createMockContext()
    );

    expect(response.status).toBe(200);
    const [, fetchOpts] = mockFetch.mock.calls[0] as [string, RequestInit];
    const sentBody = JSON.parse(fetchOpts.body as string);
    expect(sentBody.userId).toBe(pseudonymizeId(mockUser.id));
    expect(sentBody.userId).not.toBe(pseudonymizeId("victim-id"));
  });

  it("returns 400 when an anonymous user has no device id", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(resolvedURLs);
    vi.mocked(getUser).mockResolvedValue(anonUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const { DELETE } = await import("./route");
    const response = await DELETE(createDeleteRequest({}, ""), createMockContext());

    expect(response.status).toBe(400);
  });

  it("returns 503 when service URLs are not resolvable", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(null);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const { DELETE } = await import("./route");
    const response = await DELETE(createDeleteRequest({ scope: "all" }), createMockContext());

    expect(response.status).toBe(503);
  });

  it("returns 403 when user lacks workspace access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: false,
      role: null,
      permissions: noPermissions,
    });

    const { DELETE } = await import("./route");
    const response = await DELETE(createDeleteRequest({ scope: "all" }), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 502 on fetch error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

    vi.mocked(resolveServiceURLs).mockResolvedValue(resolvedURLs);
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    mockFetch.mockRejectedValueOnce(new Error("Connection refused"));

    const { DELETE } = await import("./route");
    const response = await DELETE(createDeleteRequest({ scope: "all" }), createMockContext());

    expect(response.status).toBe(502);
  });
});

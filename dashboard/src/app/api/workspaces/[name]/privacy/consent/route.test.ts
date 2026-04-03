/**
 * Tests for consent proxy route.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
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

const mockFetch = vi.fn();
global.fetch = mockFetch;

function createGetRequest(query = "?userId=user-123"): NextRequest {
  return new NextRequest(
    `http://localhost:3000/api/workspaces/test-ws/privacy/consent${query}`,
    { method: "GET" }
  );
}

function createPutRequest(body: object, query = "?userId=user-123"): NextRequest {
  return new NextRequest(
    `http://localhost:3000/api/workspaces/test-ws/privacy/consent${query}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }
  );
}

function createMockContext() {
  return { params: Promise.resolve({ name: "test-ws" }) };
}

describe("GET /api/workspaces/[name]/privacy/consent", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("SESSION_API_URL", "https://session-api:8080");
  });

  afterEach(() => {
    vi.resetAllMocks();
    vi.unstubAllEnvs();
  });

  it("proxies GET consent to session-api with userId in path", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ grants: ["analytics"], defaults: ["essential"], denied: [] }),
    });

    const { GET } = await import("./route");
    const response = await GET(createGetRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.grants).toEqual(["analytics"]);

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("/api/v1/privacy/preferences/user-123/consent");
  });

  it("returns 400 when userId is missing", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const { GET } = await import("./route");
    const response = await GET(createGetRequest(""), createMockContext());

    expect(response.status).toBe(400);
  });

  it("returns 503 when SESSION_API_URL is not set", async () => {
    vi.stubEnv("SESSION_API_URL", "");
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

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

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
      json: () => Promise.resolve({ error: "not found" }),
    });

    const { GET } = await import("./route");
    const response = await GET(createGetRequest(), createMockContext());

    expect(response.status).toBe(404);
  });
});

describe("PUT /api/workspaces/[name]/privacy/consent", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.stubEnv("SESSION_API_URL", "https://session-api:8080");
  });

  afterEach(() => {
    vi.resetAllMocks();
    vi.unstubAllEnvs();
  });

  it("proxies PUT consent to session-api with body", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const updatedConsent = {
      grants: ["analytics", "personalization"],
      defaults: ["essential"],
      denied: [],
    };
    mockFetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () => Promise.resolve(updatedConsent),
    });

    const { PUT } = await import("./route");
    const response = await PUT(
      createPutRequest({ grants: ["analytics", "personalization"] }),
      createMockContext()
    );

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.grants).toEqual(["analytics", "personalization"]);

    const [fetchUrl, fetchOpts] = mockFetch.mock.calls[0] as [string, RequestInit];
    expect(fetchUrl).toContain("/api/v1/privacy/preferences/user-123/consent");
    expect(fetchOpts.method).toBe("PUT");
    expect(fetchOpts.headers).toMatchObject({ "Content-Type": "application/json" });
  });

  it("returns 400 when userId is missing", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const { PUT } = await import("./route");
    const response = await PUT(createPutRequest({}, ""), createMockContext());

    expect(response.status).toBe(400);
  });

  it("returns 503 when SESSION_API_URL is not set", async () => {
    vi.stubEnv("SESSION_API_URL", "");
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    const { PUT } = await import("./route");
    const response = await PUT(createPutRequest({ grants: [] }), createMockContext());

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

    const { PUT } = await import("./route");
    const response = await PUT(createPutRequest({ grants: [] }), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 502 on fetch error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPermissions,
    });

    mockFetch.mockRejectedValueOnce(new Error("Connection refused"));

    const { PUT } = await import("./route");
    const response = await PUT(createPutRequest({ grants: ["analytics"] }), createMockContext());

    expect(response.status).toBe(502);
  });
});

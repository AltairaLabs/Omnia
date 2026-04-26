/**
 * Tests for the agent-memories proxy route.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

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

const viewerPermissions = { read: true, write: false, delete: false, manageMembers: false };
const noPermissions = { read: false, write: false, delete: false, manageMembers: false };

const mockWorkspace = {
  metadata: { uid: "workspace-uid-123", name: "test-ws" },
  spec: {},
};

function createMockContext() {
  return { params: Promise.resolve({ name: "test-ws" }) };
}

function createRequest(query = ""): NextRequest {
  const suffix = query ? "?" + query : "";
  return new NextRequest(
    `https://localhost:3000/api/workspaces/test-ws/agent-memories${suffix}`,
    { method: "GET" },
  );
}

function mockJsonResponse(data: unknown, status = 200) {
  mockFetch.mockResolvedValueOnce({
    ok: status >= 200 && status < 300,
    status,
    text: () => Promise.resolve(JSON.stringify(data)),
  });
}

async function authedGET(query = "") {
  const { getUser } = await import("@/lib/auth");
  const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
  const { getWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
  const { resolveServiceURLs } = await import("@/lib/k8s/service-url-resolver");

  vi.mocked(getUser).mockResolvedValue(mockUser);
  vi.mocked(checkWorkspaceAccess).mockResolvedValue({
    granted: true,
    role: "viewer",
    permissions: viewerPermissions,
  });
  vi.mocked(getWorkspace).mockResolvedValue(mockWorkspace as never);
  vi.mocked(resolveServiceURLs).mockResolvedValue({
    sessionURL: "https://session-api:8080",
    memoryURL: "https://memory-api:8080",
  });

  const { GET } = await import("./route");
  return GET(createRequest(query), createMockContext());
}

describe("GET /api/workspaces/[name]/agent-memories", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns 400 when agent query param is missing", async () => {
    const response = await authedGET("");
    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.error).toContain("agent");
  });

  it("proxies to memory-api with workspace + agent", async () => {
    const rows = {
      memories: [
        { id: "a-1", tier: "agent", scope: { agent_id: "support" } },
        { id: "a-2", tier: "agent", scope: { agent_id: "support" } },
      ],
      total: 2,
    };
    mockJsonResponse(rows);

    const response = await authedGET("agent=support-agent-uid");
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.total).toBe(2);

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("/api/v1/agent-memories");
    expect(fetchUrl).toContain("workspace=workspace-uid-123");
    expect(fetchUrl).toContain("agent=support-agent-uid");
  });

  it("forwards type / limit / offset", async () => {
    mockJsonResponse({ memories: [], total: 0 });

    await authedGET("agent=a&type=pattern&limit=5&offset=10");
    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("type=pattern");
    expect(fetchUrl).toContain("limit=5");
    expect(fetchUrl).toContain("offset=10");
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
    const response = await GET(createRequest("agent=foo"), createMockContext());

    expect(response.status).toBe(403);
  });
});

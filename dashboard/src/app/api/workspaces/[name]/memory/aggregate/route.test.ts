/**
 * Tests for the memory aggregate proxy route.
 *
 * Covers:
 *  - 400 on missing/unknown groupBy
 *  - 400 on unknown metric
 *  - happy path with proxy → backend
 *  - from/to/limit forwarded
 *  - 403 when workspace access is denied
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
    `https://localhost:3000/api/workspaces/test-ws/memory/aggregate${suffix}`,
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

describe("GET /api/workspaces/[name]/memory/aggregate", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns 400 when groupBy is missing", async () => {
    const response = await authedGET("");
    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.error).toContain("groupBy");
    expect(body.error).toContain("tier");
  });

  it("returns 400 when groupBy is unknown", async () => {
    const response = await authedGET("groupBy=banana");
    expect(response.status).toBe(400);
  });

  it("returns 400 when metric is unknown", async () => {
    const response = await authedGET("groupBy=tier&metric=banana");
    expect(response.status).toBe(400);
  });

  it("proxies tier groupBy to the backend", async () => {
    const rows = [
      { key: "institutional", value: 5, count: 5 },
      { key: "agent", value: 3, count: 3 },
      { key: "user", value: 12, count: 12 },
    ];
    mockJsonResponse(rows);

    const response = await authedGET("groupBy=tier");
    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toEqual(rows);

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("/api/v1/memories/aggregate");
    expect(fetchUrl).toContain("groupBy=tier");
    expect(fetchUrl).toContain("workspace=workspace-uid-123");
  });

  it("forwards metric, from, to, and limit params", async () => {
    mockJsonResponse([]);

    await authedGET(
      "groupBy=day&metric=count&from=2026-04-01T00:00:00Z&to=2026-04-25T00:00:00Z&limit=50",
    );

    const fetchUrl = mockFetch.mock.calls[0][0] as string;
    expect(fetchUrl).toContain("groupBy=day");
    expect(fetchUrl).toContain("metric=count");
    expect(fetchUrl).toContain("from=2026-04-01T00");
    expect(fetchUrl).toContain("to=2026-04-25T00");
    expect(fetchUrl).toContain("limit=50");
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
    const response = await GET(createRequest("groupBy=tier"), createMockContext());

    expect(response.status).toBe(403);
  });
});

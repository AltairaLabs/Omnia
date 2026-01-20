/**
 * Tests for Arena stats API route.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

// Mock dependencies before imports
vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/k8s/crd-operations", () => ({
  listCrd: vi.fn(),
  extractK8sErrorMessage: vi.fn((error: unknown) =>
    error instanceof Error ? error.message : String(error)
  ),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/workspace-route-helpers")>();
  return {
    ...actual,
    validateWorkspace: vi.fn(),
    createAuditContext: vi.fn(() => ({
      workspace: "test-ws",
      namespace: "test-ns",
      user: { username: "testuser" },
      role: "viewer",
      resourceType: "ArenaStats",
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

const mockSources = [
  { metadata: { name: "source-1" }, status: { phase: "Ready" } },
  { metadata: { name: "source-2" }, status: { phase: "Ready" } },
  { metadata: { name: "source-3" }, status: { phase: "Failed" } },
];

const mockConfigs = [
  { metadata: { name: "config-1" }, status: { phase: "Ready", scenarioCount: 10 } },
  { metadata: { name: "config-2" }, status: { phase: "Ready", scenarioCount: 5 } },
];

const mockJobs = [
  { metadata: { name: "job-1" }, status: { phase: "Running" } },
  { metadata: { name: "job-2" }, status: { phase: "Completed" } },
  { metadata: { name: "job-3" }, status: { phase: "Completed" } },
  { metadata: { name: "job-4" }, status: { phase: "Failed" } },
  { metadata: { name: "job-5" }, status: { phase: "Pending" } },
];

function createMockRequest(): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/arena/stats";
  return new NextRequest(url, { method: "GET" });
}

function createMockContext() {
  return {
    params: Promise.resolve({ name: "test-ws" }),
  };
}

describe("GET /api/workspaces/[name]/arena/stats", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns aggregated stats for authenticated user with access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });

    // Mock listCrd to return different data based on the CRD type
    vi.mocked(listCrd)
      .mockResolvedValueOnce(mockSources) // arenasources
      .mockResolvedValueOnce(mockConfigs) // arenaconfigs
      .mockResolvedValueOnce(mockJobs);    // arenajobs

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();

    // Source stats
    expect(body.sources.total).toBe(3);
    expect(body.sources.ready).toBe(2);
    expect(body.sources.failed).toBe(1);
    expect(body.sources.active).toBe(2);

    // Config stats
    expect(body.configs.total).toBe(2);
    expect(body.configs.ready).toBe(2);
    expect(body.configs.scenarios).toBe(15); // 10 + 5

    // Job stats
    expect(body.jobs.total).toBe(5);
    expect(body.jobs.running).toBe(1);
    expect(body.jobs.queued).toBe(1); // Pending
    expect(body.jobs.completed).toBe(2);
    expect(body.jobs.failed).toBe(1);
    // successRate = completed / (completed + failed) = 2 / 3 ≈ 0.667
    expect(body.jobs.successRate).toBeCloseTo(0.667, 2);
  });

  it("returns empty stats when no resources exist", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });

    vi.mocked(listCrd)
      .mockResolvedValueOnce([]) // arenasources
      .mockResolvedValueOnce([]) // arenaconfigs
      .mockResolvedValueOnce([]); // arenajobs

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();

    expect(body.sources.total).toBe(0);
    expect(body.configs.total).toBe(0);
    expect(body.jobs.total).toBe(0);
    expect(body.jobs.successRate).toBe(0);
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

  it("returns 404 when workspace not found", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace, notFoundResponse } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: false,
      response: notFoundResponse("Workspace not found: test-ws"),
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(404);
  });

  it("returns 500 on K8s error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    });
    vi.mocked(listCrd).mockRejectedValue(new Error("K8s connection failed"));

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(500);
  });
});

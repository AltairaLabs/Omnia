/**
 * Tests for Arena project jobs list API routes.
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

vi.mock("@/lib/k8s/workspace-route-helpers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/workspace-route-helpers")>();
  return {
    ...actual,
    getWorkspace: vi.fn(),
    validateWorkspace: vi.fn(),
    createAuditContext: vi.fn().mockReturnValue({}),
    auditSuccess: vi.fn(),
    auditError: vi.fn(),
  };
});

vi.mock("@/lib/k8s/crd-operations", () => ({
  listCrd: vi.fn(),
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

function createMockRequest(query: Record<string, string> = {}): NextRequest {
  const url = new URL("http://localhost:3000/api/workspaces/test-ws/arena/projects/project-1/jobs");
  for (const [key, value] of Object.entries(query)) {
    url.searchParams.set(key, value);
  }
  return new NextRequest(url.toString(), { method: "GET" });
}

function createMockContext(projectId = "project-1") {
  return {
    params: Promise.resolve({ name: "test-ws", id: projectId }),
  };
}

describe("GET /api/workspaces/[name]/arena/projects/[id]/jobs", () => {
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

  it("returns deployed:false when project is not deployed", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(listCrd).mockResolvedValue([]);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.deployed).toBe(false);
    expect(body.jobs).toEqual([]);
  });

  it("returns jobs for deployed project", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const mockSource = {
      metadata: {
        name: "project-project-1",
        namespace: "test-ns",
        labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
      },
      spec: { type: "configmap", configMap: { name: "arena-project-project-1" }, interval: "5m" },
      status: { phase: "Ready" },
    };

    const mockJobs = [
      {
        metadata: { name: "job-1", namespace: "test-ns", creationTimestamp: "2024-01-02T00:00:00Z" },
        spec: { type: "evaluation", sourceRef: { name: "project-project-1" } },
        status: { phase: "Completed" },
      },
      {
        metadata: { name: "job-2", namespace: "test-ns", creationTimestamp: "2024-01-01T00:00:00Z" },
        spec: { type: "loadtest", sourceRef: { name: "project-project-1" } },
        status: { phase: "Running" },
      },
    ];

    vi.mocked(listCrd)
      .mockResolvedValueOnce([mockSource]) // First call for sources
      .mockResolvedValueOnce(mockJobs); // Second call for jobs

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.deployed).toBe(true);
    expect(body.jobs).toHaveLength(2);
    expect(body.source).toBeDefined();
    // Jobs should be sorted by creation time, newest first
    expect(body.jobs[0].metadata.name).toBe("job-1");
    expect(body.jobs[1].metadata.name).toBe("job-2");
  });

  it("filters jobs by type", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const mockSource = {
      metadata: {
        name: "project-project-1",
        namespace: "test-ns",
        labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
      },
      spec: { type: "configmap", interval: "5m" },
      status: { phase: "Ready" },
    };

    const mockJobs = [
      {
        metadata: { name: "job-1", namespace: "test-ns", creationTimestamp: "2024-01-02T00:00:00Z" },
        spec: { type: "evaluation", sourceRef: { name: "project-project-1" } },
        status: { phase: "Completed" },
      },
      {
        metadata: { name: "job-2", namespace: "test-ns", creationTimestamp: "2024-01-01T00:00:00Z" },
        spec: { type: "loadtest", sourceRef: { name: "project-project-1" } },
        status: { phase: "Running" },
      },
    ];

    vi.mocked(listCrd)
      .mockResolvedValueOnce([mockSource])
      .mockResolvedValueOnce(mockJobs);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest({ type: "evaluation" }), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.jobs).toHaveLength(1);
    expect(body.jobs[0].spec.type).toBe("evaluation");
  });

  it("filters jobs by status", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const mockSource = {
      metadata: {
        name: "project-project-1",
        namespace: "test-ns",
        labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
      },
      spec: { type: "configmap", interval: "5m" },
      status: { phase: "Ready" },
    };

    const mockJobs = [
      {
        metadata: { name: "job-1", namespace: "test-ns", creationTimestamp: "2024-01-02T00:00:00Z" },
        spec: { type: "evaluation", sourceRef: { name: "project-project-1" } },
        status: { phase: "Completed" },
      },
      {
        metadata: { name: "job-2", namespace: "test-ns", creationTimestamp: "2024-01-01T00:00:00Z" },
        spec: { type: "loadtest", sourceRef: { name: "project-project-1" } },
        status: { phase: "Running" },
      },
    ];

    vi.mocked(listCrd)
      .mockResolvedValueOnce([mockSource])
      .mockResolvedValueOnce(mockJobs);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest({ status: "Running" }), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.jobs).toHaveLength(1);
    expect(body.jobs[0].status.phase).toBe("Running");
  });

  it("applies limit parameter", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const mockSource = {
      metadata: {
        name: "project-project-1",
        namespace: "test-ns",
        labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
      },
      spec: { type: "configmap", interval: "5m" },
      status: { phase: "Ready" },
    };

    const mockJobs = [
      {
        metadata: { name: "job-1", namespace: "test-ns", creationTimestamp: "2024-01-03T00:00:00Z" },
        spec: { type: "evaluation", sourceRef: { name: "project-project-1" } },
      },
      {
        metadata: { name: "job-2", namespace: "test-ns", creationTimestamp: "2024-01-02T00:00:00Z" },
        spec: { type: "evaluation", sourceRef: { name: "project-project-1" } },
      },
      {
        metadata: { name: "job-3", namespace: "test-ns", creationTimestamp: "2024-01-01T00:00:00Z" },
        spec: { type: "evaluation", sourceRef: { name: "project-project-1" } },
      },
    ];

    vi.mocked(listCrd)
      .mockResolvedValueOnce([mockSource])
      .mockResolvedValueOnce(mockJobs);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest({ limit: "2" }), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.jobs).toHaveLength(2);
    // Should return newest jobs
    expect(body.jobs[0].metadata.name).toBe("job-1");
    expect(body.jobs[1].metadata.name).toBe("job-2");
  });

  it("filters jobs by project label", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const mockSource = {
      metadata: {
        name: "project-project-1",
        namespace: "test-ns",
        labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
      },
      spec: { type: "configmap", interval: "5m" },
      status: { phase: "Ready" },
    };

    const mockJobs = [
      {
        metadata: {
          name: "job-1",
          namespace: "test-ns",
          creationTimestamp: "2024-01-02T00:00:00Z",
          labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
        },
        spec: { type: "evaluation", sourceRef: { name: "other-source" } }, // Different source but same project label
      },
      {
        metadata: { name: "job-2", namespace: "test-ns", creationTimestamp: "2024-01-01T00:00:00Z" },
        spec: { type: "evaluation", sourceRef: { name: "another-source" } }, // Different source, no label
      },
    ];

    vi.mocked(listCrd)
      .mockResolvedValueOnce([mockSource])
      .mockResolvedValueOnce(mockJobs);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    // Should only include job-1 which has the matching project label
    expect(body.jobs).toHaveLength(1);
    expect(body.jobs[0].metadata.name).toBe("job-1");
  });
});

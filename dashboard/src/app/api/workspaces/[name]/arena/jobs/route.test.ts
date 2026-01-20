/**
 * Tests for Arena jobs API routes.
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
  createCrd: vi.fn(),
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
      role: "editor",
      resourceType: "ArenaJob",
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
const editorPermissions = { read: true, write: true, delete: true, manageMembers: false };
const noPermissions = { read: false, write: false, delete: false, manageMembers: false };

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

function createMockRequest(method: string, body?: unknown, searchParams?: Record<string, string>): NextRequest {
  let url = "http://localhost:3000/api/workspaces/test-ws/arena/jobs";
  if (searchParams) {
    const params = new URLSearchParams(searchParams);
    url += `?${params.toString()}`;
  }
  const init: { method: string; body?: string; headers?: Record<string, string> } = { method };
  if (body) {
    init.body = JSON.stringify(body);
    init.headers = { "Content-Type": "application/json" };
  }
  return new NextRequest(url, init);
}

function createMockContext() {
  return {
    params: Promise.resolve({ name: "test-ws" }),
  };
}

describe("GET /api/workspaces/[name]/arena/jobs", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns arena jobs for authenticated user with access", async () => {
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

    const mockJobs = [
      {
        metadata: { name: "eval-job-1", namespace: "test-ns" },
        spec: { type: "evaluation", configRef: "eval-config" },
        status: { phase: "Running", completedTasks: 5, totalTasks: 10 },
      },
      {
        metadata: { name: "load-job-1", namespace: "test-ns" },
        spec: { type: "load-test", configRef: "load-config" },
        status: { phase: "Completed", completedTasks: 100, totalTasks: 100 },
      },
    ];
    vi.mocked(listCrd).mockResolvedValue(mockJobs);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toHaveLength(2);
    expect(body[0].metadata.name).toBe("eval-job-1");
  });

  it("filters jobs by type query parameter", async () => {
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

    const mockJobs = [
      {
        metadata: { name: "eval-job-1", namespace: "test-ns" },
        spec: { type: "evaluation", configRef: "eval-config" },
        status: { phase: "Running" },
      },
      {
        metadata: { name: "load-job-1", namespace: "test-ns" },
        spec: { type: "load-test", configRef: "load-config" },
        status: { phase: "Completed" },
      },
    ];
    vi.mocked(listCrd).mockResolvedValue(mockJobs);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET", undefined, { type: "evaluation" }), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toHaveLength(1);
    expect(body[0].spec.type).toBe("evaluation");
  });

  it("filters jobs by status query parameter", async () => {
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

    const mockJobs = [
      {
        metadata: { name: "eval-job-1", namespace: "test-ns" },
        spec: { type: "evaluation" },
        status: { phase: "Running" },
      },
      {
        metadata: { name: "load-job-1", namespace: "test-ns" },
        spec: { type: "load-test" },
        status: { phase: "Completed" },
      },
    ];
    vi.mocked(listCrd).mockResolvedValue(mockJobs);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET", undefined, { status: "Running" }), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toHaveLength(1);
    expect(body[0].status.phase).toBe("Running");
  });

  it("returns 403 when user lacks workspace access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: false, role: null, permissions: noPermissions });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(403);
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
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(500);
  });
});

describe("POST /api/workspaces/[name]/arena/jobs", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("creates arena job for user with editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { createCrd } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    });

    const createdJob = {
      metadata: { name: "new-job", namespace: "test-ns" },
      spec: { type: "evaluation", configRef: "eval-config" },
    };
    vi.mocked(createCrd).mockResolvedValue(createdJob);

    const { POST } = await import("./route");
    const requestBody = {
      metadata: { name: "new-job" },
      spec: { type: "evaluation", configRef: "eval-config" },
    };
    const response = await POST(createMockRequest("POST", requestBody), createMockContext());

    expect(response.status).toBe(201);
    const body = await response.json();
    expect(body.metadata.name).toBe("new-job");
  });

  it("returns 403 when user lacks editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: false, role: "viewer", permissions: viewerPermissions });

    const { POST } = await import("./route");
    const requestBody = { metadata: { name: "new-job" }, spec: {} };
    const response = await POST(createMockRequest("POST", requestBody), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 500 on create error", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { createCrd } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    });
    vi.mocked(createCrd).mockRejectedValue(new Error("Create failed"));

    const { POST } = await import("./route");
    const requestBody = { metadata: { name: "new-job" }, spec: {} };
    const response = await POST(createMockRequest("POST", requestBody), createMockContext());

    expect(response.status).toBe(500);
  });
});

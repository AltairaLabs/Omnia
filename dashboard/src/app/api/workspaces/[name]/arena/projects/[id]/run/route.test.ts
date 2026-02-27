/**
 * Tests for Arena project run API routes.
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
  createCrd: vi.fn(),
}));

const mockUser = {
  id: "testuser-id",
  provider: "oauth" as const,
  username: "testuser",
  email: "test@example.com",
  groups: ["users"],
  role: "editor" as const,
};

const editorPermissions = { read: true, write: true, delete: true, manageMembers: false };
const noPermissions = { read: false, write: false, delete: false, manageMembers: false };

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

function createMockRequest(body: object): NextRequest {
  const url = new URL("http://localhost:3000/api/workspaces/test-ws/arena/projects/project-1/run");
  return new NextRequest(url.toString(), {
    method: "POST",
    body: JSON.stringify(body),
    headers: { "Content-Type": "application/json" },
  });
}

function createMockContext(projectId = "project-1") {
  return {
    params: Promise.resolve({ name: "test-ws", id: projectId }),
  };
}

describe("POST /api/workspaces/[name]/arena/projects/[id]/run", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns 403 when user lacks editor access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: false, role: null, permissions: noPermissions });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ type: "evaluation" }), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 400 when job type is missing", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({}), createMockContext());

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("type is required");
  });

  it("returns 400 when job type is invalid", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ type: "invalid-type" }), createMockContext());

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("Invalid job type");
  });

  it("returns 404 when project is not deployed", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(listCrd).mockResolvedValue([]);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ type: "evaluation" }), createMockContext());

    expect(response.status).toBe(404);
    const body = await response.json();
    expect(body.message).toContain("not deployed");
  });

  it("returns 400 when source is not ready", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(listCrd).mockResolvedValue([
      {
        metadata: {
          name: "project-project-1",
          namespace: "test-ns",
          labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
        },
        spec: { type: "configmap", configMap: { name: "arena-project-project-1" }, interval: "5m" },
        status: { phase: "Failed" },
      },
    ]);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ type: "evaluation" }), createMockContext());

    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("not ready");
  });

  it("creates evaluation job successfully", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd, createCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(listCrd).mockResolvedValue([
      {
        metadata: {
          name: "project-project-1",
          namespace: "test-ns",
          labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
        },
        spec: { type: "configmap", configMap: { name: "arena-project-project-1" }, interval: "5m" },
        status: { phase: "Ready" },
      },
    ]);
    vi.mocked(createCrd).mockResolvedValue({
      metadata: { name: "project-1-evaluation-xyz", namespace: "test-ns" },
      spec: { type: "evaluation", sourceRef: { name: "project-project-1" } },
    });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ type: "evaluation" }), createMockContext());

    expect(response.status).toBe(201);
    const body = await response.json();
    expect(body.job).toBeDefined();
    expect(body.source).toBeDefined();
    expect(createCrd).toHaveBeenCalledWith(
      expect.anything(),
      expect.anything(),
      expect.objectContaining({
        spec: expect.objectContaining({
          type: "evaluation",
          evaluation: expect.objectContaining({
            outputFormats: ["json"],
            continueOnFailure: true,
          }),
        }),
      })
    );
  });

  it("creates loadtest job with defaults", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd, createCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(listCrd).mockResolvedValue([
      {
        metadata: {
          name: "project-project-1",
          namespace: "test-ns",
          labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
        },
        spec: { type: "configmap", configMap: { name: "arena-project-project-1" }, interval: "5m" },
        status: { phase: "Ready" },
      },
    ]);
    vi.mocked(createCrd).mockResolvedValue({
      metadata: { name: "project-1-loadtest-xyz", namespace: "test-ns" },
      spec: { type: "loadtest", sourceRef: { name: "project-project-1" } },
    });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ type: "loadtest" }), createMockContext());

    expect(response.status).toBe(201);
    expect(createCrd).toHaveBeenCalledWith(
      expect.anything(),
      expect.anything(),
      expect.objectContaining({
        spec: expect.objectContaining({
          type: "loadtest",
          loadtest: expect.objectContaining({
            profileType: "constant",
            duration: "1m",
          }),
        }),
      })
    );
  });

  it("creates datagen job with defaults", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd, createCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(listCrd).mockResolvedValue([
      {
        metadata: {
          name: "project-project-1",
          namespace: "test-ns",
          labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
        },
        spec: { type: "configmap", configMap: { name: "arena-project-project-1" }, interval: "5m" },
        status: { phase: "Ready" },
      },
    ]);
    vi.mocked(createCrd).mockResolvedValue({
      metadata: { name: "project-1-datagen-xyz", namespace: "test-ns" },
      spec: { type: "datagen", sourceRef: { name: "project-project-1" } },
    });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ type: "datagen" }), createMockContext());

    expect(response.status).toBe(201);
    expect(createCrd).toHaveBeenCalledWith(
      expect.anything(),
      expect.anything(),
      expect.objectContaining({
        spec: expect.objectContaining({
          type: "datagen",
          datagen: expect.objectContaining({
            sampleCount: 10,
            mode: "selfplay",
            outputFormat: "jsonl",
          }),
        }),
      })
    );
  });

  it("uses custom job name when provided", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd, createCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(listCrd).mockResolvedValue([
      {
        metadata: {
          name: "project-project-1",
          namespace: "test-ns",
          labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
        },
        spec: { type: "configmap", configMap: { name: "arena-project-project-1" }, interval: "5m" },
        status: { phase: "Ready" },
      },
    ]);
    vi.mocked(createCrd).mockResolvedValue({
      metadata: { name: "my-custom-job", namespace: "test-ns" },
      spec: { type: "evaluation", sourceRef: { name: "project-project-1" } },
    });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest({ type: "evaluation", name: "my-custom-job" }), createMockContext());

    expect(response.status).toBe(201);
    expect(createCrd).toHaveBeenCalledWith(
      expect.anything(),
      expect.anything(),
      expect.objectContaining({
        metadata: expect.objectContaining({
          name: "my-custom-job",
        }),
      })
    );
  });

  it("creates fleet mode evaluation with execution config", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd, createCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(listCrd).mockResolvedValue([
      {
        metadata: {
          name: "project-project-1",
          namespace: "test-ns",
          labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
        },
        spec: { type: "configmap", configMap: { name: "arena-project-project-1" }, interval: "5m" },
        status: { phase: "Ready" },
      },
    ]);
    vi.mocked(createCrd).mockResolvedValue({
      metadata: { name: "project-1-evaluation-xyz", namespace: "test-ns" },
      spec: {
        type: "evaluation",
        sourceRef: { name: "project-project-1" },
        execution: { mode: "fleet", target: { agentRuntimeRef: { name: "echo-agent" } } },
      },
    });

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest({
        type: "evaluation",
        execution: {
          mode: "fleet",
          target: { agentRuntimeRef: { name: "echo-agent" } },
        },
      }),
      createMockContext()
    );

    expect(response.status).toBe(201);
    expect(createCrd).toHaveBeenCalledWith(
      expect.anything(),
      expect.anything(),
      expect.objectContaining({
        spec: expect.objectContaining({
          type: "evaluation",
          execution: {
            mode: "fleet",
            target: { agentRuntimeRef: { name: "echo-agent" } },
          },
        }),
      })
    );
  });

  it("omits execution config for direct mode", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd, createCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(listCrd).mockResolvedValue([
      {
        metadata: {
          name: "project-project-1",
          namespace: "test-ns",
          labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
        },
        spec: { type: "configmap", configMap: { name: "arena-project-project-1" }, interval: "5m" },
        status: { phase: "Ready" },
      },
    ]);
    vi.mocked(createCrd).mockResolvedValue({
      metadata: { name: "project-1-evaluation-xyz", namespace: "test-ns" },
      spec: { type: "evaluation", sourceRef: { name: "project-project-1" } },
    });

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest({
        type: "evaluation",
        execution: { mode: "direct" },
      }),
      createMockContext()
    );

    expect(response.status).toBe(201);
    expect(createCrd).toHaveBeenCalledWith(
      expect.anything(),
      expect.anything(),
      expect.objectContaining({
        spec: expect.not.objectContaining({
          execution: expect.anything(),
        }),
      })
    );
  });

  it("includes scenarios filter when provided", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd, createCrd } = await import("@/lib/k8s/crd-operations");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    vi.mocked(listCrd).mockResolvedValue([
      {
        metadata: {
          name: "project-project-1",
          namespace: "test-ns",
          labels: { "arena.omnia.altairalabs.ai/project-id": "project-1" },
        },
        spec: { type: "configmap", configMap: { name: "arena-project-project-1" }, interval: "5m" },
        status: { phase: "Ready" },
      },
    ]);
    vi.mocked(createCrd).mockResolvedValue({
      metadata: { name: "project-1-evaluation-xyz", namespace: "test-ns" },
      spec: { type: "evaluation", sourceRef: { name: "project-project-1" } },
    });

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest({
        type: "evaluation",
        scenarios: { include: ["scenario1"], exclude: ["scenario2"] },
      }),
      createMockContext()
    );

    expect(response.status).toBe(201);
    expect(createCrd).toHaveBeenCalledWith(
      expect.anything(),
      expect.anything(),
      expect.objectContaining({
        spec: expect.objectContaining({
          scenarios: { include: ["scenario1"], exclude: ["scenario2"] },
        }),
      })
    );
  });
});

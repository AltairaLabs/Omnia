/**
 * Tests for Arena project deploy API routes.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
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
  getCrd: vi.fn(),
  createCrd: vi.fn(),
  updateCrd: vi.fn(),
}));

vi.mock("node:fs/promises", () => ({
  readdir: vi.fn(),
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

const clientOptions = { workspace: "test-ws", namespace: "test-ns", role: "editor" };

function createMockRequest(body?: object): NextRequest {
  const url = new URL("http://localhost:3000/api/workspaces/test-ws/arena/projects/project-1/deploy");
  return new NextRequest(url.toString(), {
    method: "POST",
    body: body ? JSON.stringify(body) : undefined,
    headers: body ? { "Content-Type": "application/json" } : undefined,
  });
}

function createMockContext(projectId = "project-1") {
  return { params: Promise.resolve({ name: "test-ws", id: projectId }) };
}

async function grantAccess() {
  const { getUser } = await import("@/lib/auth");
  const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
  const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
  vi.mocked(getUser).mockResolvedValue(mockUser);
  vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
  vi.mocked(validateWorkspace).mockResolvedValue({
    ok: true,
    workspace: mockWorkspace,
    clientOptions,
  } as Awaited<ReturnType<typeof validateWorkspace>>);
}

describe("POST /api/workspaces/[name]/arena/projects/[id]/deploy", () => {
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
    const response = await POST(createMockRequest(), createMockContext());
    expect(response.status).toBe(403);
  });

  it("returns 404 when the project dir does not exist", async () => {
    await grantAccess();
    const fs = await import("node:fs/promises");
    vi.mocked(fs.readdir).mockRejectedValue(new Error("ENOENT"));

    const { POST } = await import("./route");
    const response = await POST(createMockRequest(), createMockContext());
    expect(response.status).toBe(404);
  });

  it("returns 400 when the project dir is empty", async () => {
    await grantAccess();
    const fs = await import("node:fs/promises");
    vi.mocked(fs.readdir).mockResolvedValue([] as unknown as Awaited<ReturnType<typeof fs.readdir>>);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest(), createMockContext());
    expect(response.status).toBe(400);
    const body = await response.json();
    expect(body.message).toContain("no files");
  });

  it("creates a workspace ArenaSource (no ConfigMap) on first deploy", async () => {
    await grantAccess();
    const { getCrd, createCrd } = await import("@/lib/k8s/crd-operations");
    const fs = await import("node:fs/promises");
    vi.mocked(fs.readdir).mockResolvedValue(["pack.json"] as unknown as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(getCrd).mockResolvedValue(null);
    vi.mocked(createCrd).mockResolvedValue({
      metadata: { name: "project-project-1", namespace: "test-ns" },
      spec: { type: "workspace", workspace: { path: "arena/projects/project-1" } },
    });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest(), createMockContext());

    expect(response.status).toBe(201);
    const body = await response.json();
    expect(body.isNew).toBe(true);
    expect(body.configMap).toBeUndefined();
    expect(createCrd).toHaveBeenCalledWith(
      expect.anything(),
      expect.anything(),
      expect.objectContaining({
        spec: expect.objectContaining({
          type: "workspace",
          workspace: { path: "arena/projects/project-1" },
          targetPath: "arena/deployed/project-1",
        }),
      })
    );
  });

  it("replaces a stale configmap source with a workspace source on redeploy", async () => {
    await grantAccess();
    const { getCrd, updateCrd } = await import("@/lib/k8s/crd-operations");
    const fs = await import("node:fs/promises");
    vi.mocked(fs.readdir).mockResolvedValue(["pack.json"] as unknown as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(getCrd).mockResolvedValue({
      metadata: { name: "project-project-1", namespace: "test-ns", labels: {} },
      spec: { type: "configmap", configMap: { name: "arena-project-project-1" }, interval: "5m" },
    });
    vi.mocked(updateCrd).mockResolvedValue({
      metadata: { name: "project-project-1", namespace: "test-ns" },
      spec: { type: "workspace", workspace: { path: "arena/projects/project-1" } },
    });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest(), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.isNew).toBe(false);

    // The PUT body must carry the workspace block and NOT the stale configMap.
    const updateArg = vi.mocked(updateCrd).mock.calls[0][3] as { spec: Record<string, unknown> };
    expect(updateArg.spec.type).toBe("workspace");
    expect(updateArg.spec.workspace).toEqual({ path: "arena/projects/project-1" });
    expect(updateArg.spec.configMap).toBeUndefined();
    // Preserves the prior interval when none is supplied.
    expect(updateArg.spec.interval).toBe("5m");
  });

  it("uses a custom source name and sync interval from the request body", async () => {
    await grantAccess();
    const { getCrd, createCrd } = await import("@/lib/k8s/crd-operations");
    const fs = await import("node:fs/promises");
    vi.mocked(fs.readdir).mockResolvedValue(["pack.json"] as unknown as Awaited<ReturnType<typeof fs.readdir>>);
    vi.mocked(getCrd).mockResolvedValue(null);
    vi.mocked(createCrd).mockResolvedValue({
      metadata: { name: "custom-source-name", namespace: "test-ns" },
      spec: { type: "workspace", workspace: { path: "arena/projects/project-1" } },
    });

    const { POST } = await import("./route");
    const response = await POST(
      createMockRequest({ name: "custom-source-name", syncInterval: "10m" }),
      createMockContext()
    );

    expect(response.status).toBe(201);
    expect(getCrd).toHaveBeenCalledWith(expect.anything(), expect.anything(), "custom-source-name");
    const createArg = vi.mocked(createCrd).mock.calls[0][2] as { spec: { interval: string } };
    expect(createArg.spec.interval).toBe("10m");
  });
});

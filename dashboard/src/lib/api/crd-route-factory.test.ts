/**
 * Tests for the CRD route factory.
 *
 * Validates that factory-generated route handlers correctly delegate to
 * workspace helpers, CRD operations, and audit logging.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { NextRequest, NextResponse } from "next/server";

// ─── Mocks ───────────────────────────────────────────────────────────────

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-guard", () => ({
  withWorkspaceAccess: vi.fn(
    (_role: string, handler: (...args: unknown[]) => Promise<NextResponse>) =>
      async (request: NextRequest, context: { params: Promise<Record<string, string>> }) => {
        const access = { granted: true, role: "editor" };
        const user = {
          id: "user-1",
          provider: "oauth",
          username: "testuser",
          email: "test@example.com",
          groups: [],
          role: "editor",
        };
        return handler(request, context, access, user);
      }
  ),
}));

vi.mock("@/lib/k8s/crd-operations", () => ({
  listCrd: vi.fn(),
  createCrd: vi.fn(),
  updateCrd: vi.fn(),
  deleteCrd: vi.fn(),
  listSharedCrd: vi.fn(),
  getSharedCrd: vi.fn(),
  extractK8sErrorMessage: vi.fn((e: unknown) =>
    e instanceof Error ? e.message : String(e)
  ),
  isForbiddenError: vi.fn(() => false),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", () => ({
  validateWorkspace: vi.fn(),
  getWorkspaceResource: vi.fn(),
  serverErrorResponse: vi.fn((_error: unknown, message: string) =>
    NextResponse.json({ error: "Internal Server Error", message }, { status: 500 })
  ),
  notFoundResponse: vi.fn((message: string) =>
    NextResponse.json({ error: "Not Found", message }, { status: 404 })
  ),
  handleK8sError: vi.fn((_error: unknown, context: string) =>
    NextResponse.json({ error: "Internal Server Error", message: `Failed to ${context}` }, { status: 500 })
  ),
  buildCrdResource: vi.fn(
    (kind: string, workspace: string, ns: string, name: string, spec: unknown) => ({
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind,
      metadata: { name, namespace: ns, labels: { "omnia.altairalabs.ai/workspace": workspace } },
      spec,
    })
  ),
  WORKSPACE_LABEL: "omnia.altairalabs.ai/workspace",
  SYSTEM_NAMESPACE: "omnia-system",
  createAuditContext: vi.fn(
    (workspace: string, ns: string, user: unknown, role: string, resourceType: string) => ({
      workspace,
      namespace: ns,
      user,
      role,
      resourceType,
    })
  ),
  auditSuccess: vi.fn(),
  auditError: vi.fn(),
}));

vi.mock("@/lib/audit", () => ({
  logError: vi.fn(),
  logCrdSuccess: vi.fn(),
  logCrdDenied: vi.fn(),
  logCrdError: vi.fn(),
}));

// ─── Test types ──────────────────────────────────────────────────────────

interface TestResource {
  metadata?: {
    name?: string;
    namespace?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
  };
  spec?: { field: string };
}

// ─── Helpers ─────────────────────────────────────────────────────────────

import type { Workspace } from "@/types/workspace";
import type { WorkspaceResult } from "@/lib/k8s/workspace-route-helpers";

const MOCK_WORKSPACE = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "Workspace",
  metadata: { name: "test-ws" },
  spec: { namespace: { name: "test-ns" } },
} as unknown as Workspace;

function makeRequest(url: string, method = "GET", body?: unknown): NextRequest {
  const opts: { method: string; body?: string; headers?: Record<string, string> } = { method };
  if (body) {
    opts.body = JSON.stringify(body);
    opts.headers = { "Content-Type": "application/json" };
  }
  return new NextRequest(url, opts);
}

function makeContext<T extends Record<string, string>>(params: T) {
  return { params: Promise.resolve(params) } as { params: Promise<T> };
}

function validWorkspaceResult(): WorkspaceResult {
  return {
    ok: true as const,
    workspace: MOCK_WORKSPACE,
    clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" as const },
  };
}

function invalidWorkspaceResult(): WorkspaceResult {
  return {
    ok: false as const,
    response: NextResponse.json({ error: "Not Found" }, { status: 404 }),
  };
}

// ─── Tests ───────────────────────────────────────────────────────────────

describe("createCollectionRoutes", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("GET lists resources and returns 200", async () => {
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    const { createCollectionRoutes } = await import("./crd-route-factory");

    vi.mocked(validateWorkspace).mockResolvedValue(validWorkspaceResult());
    const mockItems = [{ metadata: { name: "item-1" }, spec: { field: "a" } }];
    vi.mocked(listCrd).mockResolvedValue(mockItems);

    const { GET } = createCollectionRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      errorLabel: "test resources",
    });

    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds");
    const context = makeContext({ name: "test-ws" });
    const response = await GET(request, context);

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toHaveLength(1);
    expect(body[0].metadata.name).toBe("item-1");
    expect(listCrd).toHaveBeenCalledWith(
      expect.objectContaining({ namespace: "test-ns" }),
      "testkinds"
    );
  });

  it("GET returns workspace validation error if workspace not found", async () => {
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { createCollectionRoutes } = await import("./crd-route-factory");

    vi.mocked(validateWorkspace).mockResolvedValue(invalidWorkspaceResult());

    const { GET } = createCollectionRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      errorLabel: "test resources",
    });

    const request = makeRequest("http://localhost/api/workspaces/bad-ws/testkinds");
    const context = makeContext({ name: "bad-ws" });
    const response = await GET(request, context);

    expect(response.status).toBe(404);
  });

  it("GET returns 500 and logs audit error on failure", async () => {
    const { validateWorkspace, auditError } = await import("@/lib/k8s/workspace-route-helpers");
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    const { createCollectionRoutes } = await import("./crd-route-factory");

    vi.mocked(validateWorkspace).mockResolvedValue(validWorkspaceResult());
    vi.mocked(listCrd).mockRejectedValue(new Error("K8s connection failed"));

    const { GET } = createCollectionRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      errorLabel: "test resources",
    });

    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds");
    const context = makeContext({ name: "test-ws" });
    const response = await GET(request, context);

    expect(response.status).toBe(500);
    expect(auditError).toHaveBeenCalledWith(
      expect.objectContaining({ resourceType: "TestKind" }),
      "list",
      undefined,
      expect.any(Error),
      500
    );
  });

  it("POST creates a resource and returns 201", async () => {
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { createCrd } = await import("@/lib/k8s/crd-operations");
    const { createCollectionRoutes } = await import("./crd-route-factory");

    vi.mocked(validateWorkspace).mockResolvedValue(validWorkspaceResult());
    const created = { metadata: { name: "new-item" }, spec: { field: "b" } };
    vi.mocked(createCrd).mockResolvedValue(created);

    const { POST } = createCollectionRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      errorLabel: "test resources",
    });

    const body = { metadata: { name: "new-item" }, spec: { field: "b" } };
    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds", "POST", body);
    const context = makeContext({ name: "test-ws" });
    const response = await POST(request, context);

    expect(response.status).toBe(201);
    const respBody = await response.json();
    expect(respBody.metadata.name).toBe("new-item");
  });

  it("POST returns workspace validation error if workspace not found", async () => {
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { createCollectionRoutes } = await import("./crd-route-factory");

    vi.mocked(validateWorkspace).mockResolvedValue(invalidWorkspaceResult());

    const { POST } = createCollectionRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      errorLabel: "test resources",
    });

    const body = { metadata: { name: "new-item" }, spec: { field: "b" } };
    const request = makeRequest("http://localhost/api/workspaces/bad-ws/testkinds", "POST", body);
    const context = makeContext({ name: "bad-ws" });
    const response = await POST(request, context);

    expect(response.status).toBe(404);
  });

  it("POST returns 500 and logs audit error on failure", async () => {
    const { validateWorkspace, auditError } = await import("@/lib/k8s/workspace-route-helpers");
    const { createCrd } = await import("@/lib/k8s/crd-operations");
    const { createCollectionRoutes } = await import("./crd-route-factory");

    vi.mocked(validateWorkspace).mockResolvedValue(validWorkspaceResult());
    vi.mocked(createCrd).mockRejectedValue(new Error("Create failed"));

    const { POST } = createCollectionRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      errorLabel: "test resources",
    });

    const body = { metadata: { name: "new-item" }, spec: { field: "b" } };
    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds", "POST", body);
    const context = makeContext({ name: "test-ws" });
    const response = await POST(request, context);

    expect(response.status).toBe(500);
    expect(auditError).toHaveBeenCalledWith(
      expect.objectContaining({ resourceType: "TestKind" }),
      "create",
      "new-item",
      expect.any(Error),
      500
    );
  });

  it("POST extracts resource name from body.name fallback", async () => {
    const { validateWorkspace, auditSuccess } = await import("@/lib/k8s/workspace-route-helpers");
    const { createCrd } = await import("@/lib/k8s/crd-operations");
    const { createCollectionRoutes } = await import("./crd-route-factory");

    vi.mocked(validateWorkspace).mockResolvedValue(validWorkspaceResult());
    vi.mocked(createCrd).mockResolvedValue({ metadata: { name: "fallback" } });

    const { POST } = createCollectionRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      errorLabel: "test resources",
    });

    const body = { name: "fallback", spec: { field: "c" } };
    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds", "POST", body);
    const context = makeContext({ name: "test-ws" });
    await POST(request, context);

    expect(auditSuccess).toHaveBeenCalledWith(
      expect.anything(),
      "create",
      "fallback"
    );
  });
});

describe("createItemRoutes", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("GET fetches a single resource and returns 200", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const { createItemRoutes } = await import("./crd-route-factory");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: { metadata: { name: "my-item" }, spec: { field: "x" } },
      workspace: MOCK_WORKSPACE,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" as const },
    });

    const { GET } = createItemRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      resourceLabel: "Test item",
      paramKey: "itemName",
      errorLabel: "this test item",
    });

    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds/my-item");
    const context = makeContext({ name: "test-ws", itemName: "my-item" });
    const response = await GET(request, context);

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.metadata.name).toBe("my-item");
  });

  it("GET returns validation error when resource not found", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const { createItemRoutes } = await import("./crd-route-factory");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: false,
      response: NextResponse.json({ error: "Not Found" }, { status: 404 }),
    });

    const { GET } = createItemRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      resourceLabel: "Test item",
      paramKey: "itemName",
      errorLabel: "this test item",
    });

    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds/missing");
    const context = makeContext({ name: "test-ws", itemName: "missing" });
    const response = await GET(request, context);

    expect(response.status).toBe(404);
  });

  it("GET returns 500 and logs audit error on exception", async () => {
    const { getWorkspaceResource, auditError } = await import("@/lib/k8s/workspace-route-helpers");
    const { createItemRoutes } = await import("./crd-route-factory");

    // Simulate getWorkspaceResource throwing an error
    vi.mocked(getWorkspaceResource).mockRejectedValue(new Error("K8s connection failed"));

    const { GET } = createItemRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      resourceLabel: "Test item",
      paramKey: "itemName",
      errorLabel: "this test item",
    });

    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds/my-item");
    const context = makeContext({ name: "test-ws", itemName: "my-item" });
    const response = await GET(request, context);

    expect(response.status).toBe(500);
    // auditCtx is not set when getWorkspaceResource throws, so auditError is not called
    expect(auditError).not.toHaveBeenCalled();
  });

  it("GET returns 500 and logs audit error when error occurs after audit context is created", async () => {
    const { getWorkspaceResource, auditSuccess, auditError } = await import("@/lib/k8s/workspace-route-helpers");
    const { createItemRoutes } = await import("./crd-route-factory");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: { metadata: { name: "my-item" }, spec: { field: "x" } },
      workspace: MOCK_WORKSPACE,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" as const },
    });

    // Force auditSuccess to throw to trigger the catch block with auditCtx set
    vi.mocked(auditSuccess).mockImplementationOnce(() => {
      throw new Error("Unexpected audit failure");
    });

    const { GET } = createItemRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      resourceLabel: "Test item",
      paramKey: "itemName",
      errorLabel: "this test item",
    });

    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds/my-item");
    const context = makeContext({ name: "test-ws", itemName: "my-item" });
    const response = await GET(request, context);

    expect(response.status).toBe(500);
    expect(auditError).toHaveBeenCalled();
  });

  it("PUT updates a resource and returns 200", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const { updateCrd } = await import("@/lib/k8s/crd-operations");
    const { createItemRoutes } = await import("./crd-route-factory");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: { metadata: { name: "my-item", labels: { existing: "label" } }, spec: { field: "old" } },
      workspace: MOCK_WORKSPACE,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" as const },
    });
    vi.mocked(updateCrd).mockResolvedValue({ metadata: { name: "my-item" }, spec: { field: "new" } });

    const { PUT } = createItemRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      resourceLabel: "Test item",
      paramKey: "itemName",
      errorLabel: "this test item",
    });

    const body = { spec: { field: "new" }, metadata: { labels: { added: "true" } } };
    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds/my-item", "PUT", body);
    const context = makeContext({ name: "test-ws", itemName: "my-item" });
    const response = await PUT(request, context);

    expect(response.status).toBe(200);
    expect(updateCrd).toHaveBeenCalledWith(
      expect.objectContaining({ namespace: "test-ns" }),
      "testkinds",
      "my-item",
      expect.objectContaining({
        metadata: expect.objectContaining({
          labels: expect.objectContaining({
            existing: "label",
            added: "true",
            "omnia.altairalabs.ai/workspace": "test-ws",
          }),
        }),
        spec: { field: "new" },
      })
    );
  });

  it("PUT returns validation error when resource not found", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const { createItemRoutes } = await import("./crd-route-factory");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: false,
      response: NextResponse.json({ error: "Not Found" }, { status: 404 }),
    });

    const { PUT } = createItemRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      resourceLabel: "Test item",
      paramKey: "itemName",
      errorLabel: "this test item",
    });

    const body = { spec: { field: "new" } };
    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds/missing", "PUT", body);
    const context = makeContext({ name: "test-ws", itemName: "missing" });
    const response = await PUT(request, context);

    expect(response.status).toBe(404);
  });

  it("PUT returns 500 and logs audit error on failure", async () => {
    const { getWorkspaceResource, auditError } = await import("@/lib/k8s/workspace-route-helpers");
    const { updateCrd } = await import("@/lib/k8s/crd-operations");
    const { createItemRoutes } = await import("./crd-route-factory");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: { metadata: { name: "my-item" }, spec: { field: "old" } },
      workspace: MOCK_WORKSPACE,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" as const },
    });
    vi.mocked(updateCrd).mockRejectedValue(new Error("Update failed"));

    const { PUT } = createItemRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      resourceLabel: "Test item",
      paramKey: "itemName",
      errorLabel: "this test item",
    });

    const body = { spec: { field: "new" } };
    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds/my-item", "PUT", body);
    const context = makeContext({ name: "test-ws", itemName: "my-item" });
    const response = await PUT(request, context);

    expect(response.status).toBe(500);
    expect(auditError).toHaveBeenCalledWith(
      expect.objectContaining({ resourceType: "TestKind" }),
      "update",
      "my-item",
      expect.any(Error),
      500
    );
  });

  it("PUT falls back to existing spec when body.spec is not provided", async () => {
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
    const { updateCrd } = await import("@/lib/k8s/crd-operations");
    const { createItemRoutes } = await import("./crd-route-factory");

    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      resource: { metadata: { name: "my-item" }, spec: { field: "original" } },
      workspace: MOCK_WORKSPACE,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" as const },
    });
    vi.mocked(updateCrd).mockResolvedValue({ metadata: { name: "my-item" }, spec: { field: "original" } });

    const { PUT } = createItemRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      resourceLabel: "Test item",
      paramKey: "itemName",
      errorLabel: "this test item",
    });

    // Body without spec field
    const body = { metadata: { labels: { added: "true" } } };
    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds/my-item", "PUT", body);
    const context = makeContext({ name: "test-ws", itemName: "my-item" });
    const response = await PUT(request, context);

    expect(response.status).toBe(200);
    expect(updateCrd).toHaveBeenCalledWith(
      expect.anything(),
      "testkinds",
      "my-item",
      expect.objectContaining({ spec: { field: "original" } })
    );
  });

  it("PUT handles error before audit context is created", async () => {
    const { getWorkspaceResource, auditError } = await import("@/lib/k8s/workspace-route-helpers");
    const { createItemRoutes } = await import("./crd-route-factory");

    vi.mocked(getWorkspaceResource).mockRejectedValue(new Error("Connection lost"));

    const { PUT } = createItemRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      resourceLabel: "Test item",
      paramKey: "itemName",
      errorLabel: "this test item",
    });

    const body = { spec: { field: "new" } };
    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds/my-item", "PUT", body);
    const context = makeContext({ name: "test-ws", itemName: "my-item" });
    const response = await PUT(request, context);

    expect(response.status).toBe(500);
    // auditError should NOT be called since auditCtx was never set
    expect(auditError).not.toHaveBeenCalled();
  });

  it("DELETE removes a resource and returns 204", async () => {
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { deleteCrd } = await import("@/lib/k8s/crd-operations");
    const { createItemRoutes } = await import("./crd-route-factory");

    vi.mocked(validateWorkspace).mockResolvedValue(validWorkspaceResult());
    vi.mocked(deleteCrd).mockResolvedValue(undefined);

    const { DELETE } = createItemRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      resourceLabel: "Test item",
      paramKey: "itemName",
      errorLabel: "this test item",
    });

    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds/my-item", "DELETE");
    const context = makeContext({ name: "test-ws", itemName: "my-item" });
    const response = await DELETE(request, context);

    expect(response.status).toBe(204);
    expect(deleteCrd).toHaveBeenCalledWith(
      expect.objectContaining({ namespace: "test-ns" }),
      "testkinds",
      "my-item"
    );
  });

  it("DELETE returns workspace validation error if workspace not found", async () => {
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { createItemRoutes } = await import("./crd-route-factory");

    vi.mocked(validateWorkspace).mockResolvedValue(invalidWorkspaceResult());

    const { DELETE } = createItemRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      resourceLabel: "Test item",
      paramKey: "itemName",
      errorLabel: "this test item",
    });

    const request = makeRequest("http://localhost/api/workspaces/bad-ws/testkinds/my-item", "DELETE");
    const context = makeContext({ name: "bad-ws", itemName: "my-item" });
    const response = await DELETE(request, context);

    expect(response.status).toBe(404);
  });

  it("DELETE handles error before audit context is created", async () => {
    const { validateWorkspace, auditError } = await import("@/lib/k8s/workspace-route-helpers");
    const { createItemRoutes } = await import("./crd-route-factory");

    vi.mocked(validateWorkspace).mockRejectedValue(new Error("Connection lost"));

    const { DELETE } = createItemRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      resourceLabel: "Test item",
      paramKey: "itemName",
      errorLabel: "this test item",
    });

    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds/my-item", "DELETE");
    const context = makeContext({ name: "test-ws", itemName: "my-item" });
    const response = await DELETE(request, context);

    expect(response.status).toBe(500);
    expect(auditError).not.toHaveBeenCalled();
  });

  it("DELETE returns 500 and logs audit error on failure", async () => {
    const { validateWorkspace, auditError } = await import("@/lib/k8s/workspace-route-helpers");
    const { deleteCrd } = await import("@/lib/k8s/crd-operations");
    const { createItemRoutes } = await import("./crd-route-factory");

    vi.mocked(validateWorkspace).mockResolvedValue(validWorkspaceResult());
    vi.mocked(deleteCrd).mockRejectedValue(new Error("Delete failed"));

    const { DELETE } = createItemRoutes<TestResource>({
      kind: "TestKind",
      plural: "testkinds",
      resourceLabel: "Test item",
      paramKey: "itemName",
      errorLabel: "this test item",
    });

    const request = makeRequest("http://localhost/api/workspaces/test-ws/testkinds/my-item", "DELETE");
    const context = makeContext({ name: "test-ws", itemName: "my-item" });
    const response = await DELETE(request, context);

    expect(response.status).toBe(500);
    expect(auditError).toHaveBeenCalledWith(
      expect.objectContaining({ resourceType: "TestKind" }),
      "delete",
      "my-item",
      expect.any(Error),
      500
    );
  });
});

describe("createSharedCollectionRoutes", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("GET lists shared resources and returns 200", async () => {
    const { listSharedCrd } = await import("@/lib/k8s/crd-operations");
    const { createSharedCollectionRoutes } = await import("./crd-route-factory");

    const mockItems = [{ metadata: { name: "shared-1" } }];
    vi.mocked(listSharedCrd).mockResolvedValue(mockItems);

    const { GET } = createSharedCollectionRoutes<TestResource>({
      plural: "testkinds",
      errorLabel: "test resources",
    });

    const response = await GET();

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toHaveLength(1);
    expect(listSharedCrd).toHaveBeenCalledWith("testkinds", "omnia-system");
  });

  it("GET returns 500 on error", async () => {
    const { listSharedCrd } = await import("@/lib/k8s/crd-operations");
    const { createSharedCollectionRoutes } = await import("./crd-route-factory");

    vi.mocked(listSharedCrd).mockRejectedValue(new Error("Failed"));

    const { GET } = createSharedCollectionRoutes<TestResource>({
      plural: "testkinds",
      errorLabel: "test resources",
    });

    const response = await GET();

    expect(response.status).toBe(500);
  });
});

describe("createSharedItemRoutes", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("GET fetches a shared resource and returns 200", async () => {
    const { getSharedCrd } = await import("@/lib/k8s/crd-operations");
    const { createSharedItemRoutes } = await import("./crd-route-factory");

    const mockItem = { metadata: { name: "shared-1" }, spec: { field: "a" } };
    vi.mocked(getSharedCrd).mockResolvedValue(mockItem);

    const { GET } = createSharedItemRoutes<TestResource>({
      plural: "testkinds",
      paramKey: "itemName",
      resourceLabel: "Test item",
      errorLabel: "test item",
    });

    const request = makeRequest("http://localhost/api/shared/testkinds/shared-1");
    const context = { params: Promise.resolve({ itemName: "shared-1" }) };
    const response = await GET(request, context);

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.metadata.name).toBe("shared-1");
    expect(getSharedCrd).toHaveBeenCalledWith("testkinds", "omnia-system", "shared-1");
  });

  it("GET returns 404 when resource not found", async () => {
    const { getSharedCrd } = await import("@/lib/k8s/crd-operations");
    const { createSharedItemRoutes } = await import("./crd-route-factory");

    vi.mocked(getSharedCrd).mockResolvedValue(null);

    const { GET } = createSharedItemRoutes<TestResource>({
      plural: "testkinds",
      paramKey: "itemName",
      resourceLabel: "Test item",
      errorLabel: "test item",
    });

    const request = makeRequest("http://localhost/api/shared/testkinds/missing");
    const context = { params: Promise.resolve({ itemName: "missing" }) };
    const response = await GET(request, context);

    expect(response.status).toBe(404);
  });

  it("GET returns 500 on error", async () => {
    const { getSharedCrd } = await import("@/lib/k8s/crd-operations");
    const { createSharedItemRoutes } = await import("./crd-route-factory");

    vi.mocked(getSharedCrd).mockRejectedValue(new Error("Failed"));

    const { GET } = createSharedItemRoutes<TestResource>({
      plural: "testkinds",
      paramKey: "itemName",
      resourceLabel: "Test item",
      errorLabel: "test item",
    });

    const request = makeRequest("http://localhost/api/shared/testkinds/shared-1");
    const context = { params: Promise.resolve({ itemName: "shared-1" }) };
    const response = await GET(request, context);

    expect(response.status).toBe(500);
  });
});

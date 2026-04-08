# Deploy Adapter Dashboard API — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the dashboard API gaps needed for the PromptArena deploy adapter to manage Omnia resources via the dashboard backend.

**Architecture:** The dashboard already has a CRD route factory (`crd-route-factory.ts`) that generates CRUD handlers. Most tasks are wiring new routes using this factory, plus adding `labelSelector` support to `listCrd()`. TypeScript types for AgentPolicy/ToolPolicy are hand-written in `dashboard/src/types/` following existing patterns. OpenAPI spec updated to document all new/changed endpoints.

**Tech Stack:** Next.js API routes, Vitest, `@kubernetes/client-node`, OpenAPI 3.0.3

**Proposal doc:** `docs/local-backlog/promptarena-omnia-deploy-adapter.md`

---

## File Structure

| Action | File | Purpose |
|--------|------|---------|
| Modify | `dashboard/src/app/api/workspaces/[name]/toolregistries/route.ts` | Export POST |
| Create | `dashboard/src/app/api/workspaces/[name]/toolregistries/route.test.ts` | Tests for GET + POST |
| Modify | `dashboard/src/app/api/workspaces/[name]/toolregistries/[registryName]/route.ts` | Export PUT + DELETE |
| Create | `dashboard/src/app/api/workspaces/[name]/toolregistries/[registryName]/route.test.ts` | Tests for GET + PUT + DELETE |
| Modify | `dashboard/src/lib/k8s/crd-operations.ts` | Add `labelSelector` param to `listCrd` |
| Modify | `dashboard/src/lib/k8s/crd-operations.test.ts` | Test `labelSelector` passthrough |
| Modify | `dashboard/src/lib/api/crd-route-factory.ts` | Parse `labelSelector` query param in GET handler |
| Modify | `dashboard/src/lib/k8s/workspace-route-helpers.ts` | Add CRD plural constants for new types |
| Create | `dashboard/src/types/agentpolicy.ts` | AgentPolicy TypeScript types |
| Create | `dashboard/src/types/toolpolicy.ts` | ToolPolicy TypeScript types |
| Modify | `dashboard/src/types/generated/index.ts` | Re-export new types |
| Create | `dashboard/src/app/api/workspaces/[name]/agentpolicies/route.ts` | GET + POST |
| Create | `dashboard/src/app/api/workspaces/[name]/agentpolicies/route.test.ts` | Tests |
| Create | `dashboard/src/app/api/workspaces/[name]/agentpolicies/[policyName]/route.ts` | GET + PUT + DELETE |
| Create | `dashboard/src/app/api/workspaces/[name]/agentpolicies/[policyName]/route.test.ts` | Tests |
| Create | `dashboard/src/app/api/workspaces/[name]/toolpolicies/route.ts` | GET + POST |
| Create | `dashboard/src/app/api/workspaces/[name]/toolpolicies/route.test.ts` | Tests |
| Create | `dashboard/src/app/api/workspaces/[name]/toolpolicies/[policyName]/route.ts` | GET + PUT + DELETE |
| Create | `dashboard/src/app/api/workspaces/[name]/toolpolicies/[policyName]/route.test.ts` | Tests |
| Modify | `api/openapi/openapi.yaml` | Add new endpoints + schemas |

---

### Task 1: Add `labelSelector` support to `listCrd`

The deploy adapter needs to query resources by label (e.g., `arena.omnia.altairalabs.ai/project-id`). The K8s `listNamespacedCustomObject` API supports `labelSelector` but `listCrd` doesn't pass it through.

**Files:**
- Modify: `dashboard/src/lib/k8s/crd-operations.ts:37-52`
- Modify: `dashboard/src/lib/k8s/crd-operations.test.ts`

- [ ] **Step 1: Write failing test for `listCrd` with `labelSelector`**

In `dashboard/src/lib/k8s/crd-operations.test.ts`, add a test to the existing `listCrd` describe block:

```typescript
it("passes labelSelector to K8s API when provided", async () => {
  mockCustomObjectsApi.listNamespacedCustomObject.mockResolvedValue({
    items: [{ metadata: { name: "filtered-agent" } }],
  });

  const result = await listCrd(
    defaultOptions,
    "agentruntimes",
    { labelSelector: "app=test" }
  );

  expect(result).toHaveLength(1);
  expect(
    mockCustomObjectsApi.listNamespacedCustomObject
  ).toHaveBeenCalledWith(
    expect.objectContaining({
      labelSelector: "app=test",
    })
  );
});

it("omits labelSelector from K8s API when not provided", async () => {
  mockCustomObjectsApi.listNamespacedCustomObject.mockResolvedValue({
    items: [],
  });

  await listCrd(defaultOptions, "agentruntimes");

  expect(
    mockCustomObjectsApi.listNamespacedCustomObject
  ).toHaveBeenCalledWith(
    expect.not.objectContaining({
      labelSelector: expect.anything(),
    })
  );
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd dashboard && npx vitest run src/lib/k8s/crd-operations.test.ts`
Expected: FAIL — `listCrd` doesn't accept a third argument.

- [ ] **Step 3: Update `listCrd` to accept optional `labelSelector`**

In `dashboard/src/lib/k8s/crd-operations.ts`, change the `listCrd` function:

```typescript
/**
 * Options for listing CRD resources.
 */
export interface ListCrdOptions {
  /** Kubernetes label selector string (e.g., "app=test,env=prod") */
  labelSelector?: string;
}

/**
 * List CRD resources in a workspace namespace.
 *
 * @param options - Workspace client options
 * @param plural - CRD plural name (e.g., "agentruntimes")
 * @param listOptions - Optional filtering (label selectors, etc.)
 * @returns Array of CRD resources
 */
export async function listCrd<T>(
  options: WorkspaceClientOptions,
  plural: string,
  listOptions?: ListCrdOptions
): Promise<T[]> {
  return withTokenRefresh(options, async () => {
    const api = await getWorkspaceCustomObjectsApi(options);
    const params: Record<string, unknown> = {
      group: CRD_GROUP,
      version: CRD_VERSION,
      namespace: options.namespace,
      plural,
    };
    if (listOptions?.labelSelector) {
      params.labelSelector = listOptions.labelSelector;
    }
    const result = await api.listNamespacedCustomObject(params);
    const list = result as { items?: T[] };
    return (list.items || []) as T[];
  });
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd dashboard && npx vitest run src/lib/k8s/crd-operations.test.ts`
Expected: PASS

- [ ] **Step 5: Propagate `labelSelector` through the CRD route factory GET handler**

In `dashboard/src/lib/api/crd-route-factory.ts`, update the GET handler inside `createCollectionRoutes` to parse `labelSelector` from the query string and pass it to `listCrd`:

```typescript
// Inside the GET handler of createCollectionRoutes, replace the listCrd call:
const labelSelector = _request.nextUrl.searchParams.get("labelSelector") || undefined;

const items = await listCrd<T>(result.clientOptions, plural, {
  ...(labelSelector && { labelSelector }),
});
```

The `_request` parameter name needs to be renamed to `request` since we now use it. Update the parameter name in the GET handler signature from `_request` to `request`.

- [ ] **Step 6: Run full test suite for affected files**

Run: `cd dashboard && npx vitest run src/lib/k8s/crd-operations.test.ts src/lib/api/crd-route-factory.test.ts`
Expected: PASS (existing tests should still pass since `labelSelector` is optional)

- [ ] **Step 7: Commit**

```
git add dashboard/src/lib/k8s/crd-operations.ts dashboard/src/lib/k8s/crd-operations.test.ts dashboard/src/lib/api/crd-route-factory.ts
```
```
feat(dashboard): add labelSelector support to CRD list endpoints

The deploy adapter needs to query resources by label. Add optional
labelSelector parameter to listCrd() and parse it from query string
in the CRD route factory GET handler.
```

---

### Task 2: Enable ToolRegistry write operations

ToolRegistry routes currently only export GET. The factory already generates POST/PUT/DELETE handlers — just export them.

**Files:**
- Modify: `dashboard/src/app/api/workspaces/[name]/toolregistries/route.ts`
- Create: `dashboard/src/app/api/workspaces/[name]/toolregistries/route.test.ts`
- Modify: `dashboard/src/app/api/workspaces/[name]/toolregistries/[registryName]/route.ts`
- Create: `dashboard/src/app/api/workspaces/[name]/toolregistries/[registryName]/route.test.ts`

- [ ] **Step 1: Write failing test for POST on collection route**

Create `dashboard/src/app/api/workspaces/[name]/toolregistries/route.test.ts`:

```typescript
/**
 * Tests for workspace-scoped tool registry collection API routes.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

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
  isForbiddenError: vi.fn(),
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
      resourceType: "ToolRegistry",
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

const editorPermissions = { read: true, write: true, delete: true, manageMembers: false };
const viewerPermissions = { read: true, write: false, delete: false, manageMembers: false };
const noPermissions = { read: false, write: false, delete: false, manageMembers: false };

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

function createMockRequest(method: string, body?: unknown): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/toolregistries";
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

describe("GET /api/workspaces/[name]/toolregistries", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("returns tool registries for authenticated user with access", async () => {
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

    const mockRegistries = [
      {
        metadata: { name: "search-tools", namespace: "test-ns" },
        spec: { tools: [{ name: "web-search" }] },
      },
    ];
    vi.mocked(listCrd).mockResolvedValue(mockRegistries);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toHaveLength(1);
    expect(body[0].metadata.name).toBe("search-tools");
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

describe("POST /api/workspaces/[name]/toolregistries", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("creates tool registry for user with editor role", async () => {
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

    const createdRegistry = {
      metadata: { name: "new-tools", namespace: "test-ns" },
      spec: { tools: [{ name: "calculator", type: "http" }] },
    };
    vi.mocked(createCrd).mockResolvedValue(createdRegistry);

    const body = {
      metadata: { name: "new-tools" },
      spec: { tools: [{ name: "calculator", type: "http" }] },
    };

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", body), createMockContext());

    expect(response.status).toBe(201);
    const result = await response.json();
    expect(result.metadata.name).toBe("new-tools");
  });

  it("returns 403 when user lacks editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });

    const body = {
      metadata: { name: "new-tools" },
      spec: { tools: [] },
    };

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", body), createMockContext());

    expect(response.status).toBe(403);
  });

  it("returns 500 on K8s error", async () => {
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
    vi.mocked(createCrd).mockRejectedValue(new Error("K8s create failed"));

    const body = {
      metadata: { name: "new-tools" },
      spec: { tools: [] },
    };

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", body), createMockContext());

    expect(response.status).toBe(500);
  });
});
```

- [ ] **Step 2: Run test to verify POST fails**

Run: `cd dashboard && npx vitest run src/app/api/workspaces/\\[name\\]/toolregistries/route.test.ts`
Expected: FAIL — `POST` is not exported from `route.ts`.

- [ ] **Step 3: Export POST from collection route**

Replace the full contents of `dashboard/src/app/api/workspaces/[name]/toolregistries/route.ts`:

```typescript
/**
 * API route for workspace-scoped tool registries.
 *
 * GET /api/workspaces/:name/toolregistries - List tool registries in workspace
 * POST /api/workspaces/:name/toolregistries - Create a new tool registry
 *
 * Tool registries can be workspace-scoped (in workspace namespace) or
 * shared (in omnia-system namespace). This endpoint manages workspace-scoped ones.
 */

import { createCollectionRoutes } from "@/lib/api/crd-route-factory";
import type { ToolRegistry } from "@/lib/data/types";

export const { GET, POST } = createCollectionRoutes<ToolRegistry>({
  kind: "ToolRegistry",
  plural: "toolregistries",
  errorLabel: "tool registries",
});
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd dashboard && npx vitest run src/app/api/workspaces/\\[name\\]/toolregistries/route.test.ts`
Expected: PASS

- [ ] **Step 5: Write failing test for PUT/DELETE on item route**

Create `dashboard/src/app/api/workspaces/[name]/toolregistries/[registryName]/route.test.ts`:

```typescript
/**
 * Tests for workspace-scoped tool registry item API routes.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/k8s/crd-operations", () => ({
  getCrd: vi.fn(),
  updateCrd: vi.fn(),
  deleteCrd: vi.fn(),
  extractK8sErrorMessage: vi.fn((error: unknown) =>
    error instanceof Error ? error.message : String(error)
  ),
  isForbiddenError: vi.fn(),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/workspace-route-helpers")>();
  return {
    ...actual,
    validateWorkspace: vi.fn(),
    getWorkspaceResource: vi.fn(),
    createAuditContext: vi.fn(() => ({
      workspace: "test-ws",
      namespace: "test-ns",
      user: { username: "testuser" },
      role: "editor",
      resourceType: "ToolRegistry",
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

const editorPermissions = { read: true, write: true, delete: true, manageMembers: false };
const viewerPermissions = { read: true, write: false, delete: false, manageMembers: false };

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

const mockRegistry = {
  metadata: {
    name: "search-tools",
    namespace: "test-ns",
    labels: { "omnia.altairalabs.ai/workspace": "test-ws" },
  },
  spec: { tools: [{ name: "web-search", type: "http" }] },
};

function createMockRequest(method: string, body?: unknown): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/toolregistries/search-tools";
  const init: { method: string; body?: string; headers?: Record<string, string> } = { method };
  if (body) {
    init.body = JSON.stringify(body);
    init.headers = { "Content-Type": "application/json" };
  }
  return new NextRequest(url, init);
}

function createMockContext() {
  return {
    params: Promise.resolve({ name: "test-ws", registryName: "search-tools" }),
  };
}

describe("GET /api/workspaces/[name]/toolregistries/[registryName]", () => {
  beforeEach(() => { vi.resetModules(); });
  afterEach(() => { vi.resetAllMocks(); });

  it("returns tool registry for authenticated user", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      resource: mockRegistry,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.metadata.name).toBe("search-tools");
  });
});

describe("PUT /api/workspaces/[name]/toolregistries/[registryName]", () => {
  beforeEach(() => { vi.resetModules(); });
  afterEach(() => { vi.resetAllMocks(); });

  it("updates tool registry for user with editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { updateCrd } = await import("@/lib/k8s/crd-operations");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
      resource: mockRegistry,
    });

    const updatedRegistry = {
      ...mockRegistry,
      spec: { tools: [{ name: "web-search", type: "http" }, { name: "calculator", type: "http" }] },
    };
    vi.mocked(updateCrd).mockResolvedValue(updatedRegistry);

    const body = {
      spec: { tools: [{ name: "web-search", type: "http" }, { name: "calculator", type: "http" }] },
    };

    const { PUT } = await import("./route");
    const response = await PUT(createMockRequest("PUT", body), createMockContext());

    expect(response.status).toBe(200);
    const result = await response.json();
    expect(result.spec.tools).toHaveLength(2);
  });

  it("returns 403 when user lacks editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });

    const { PUT } = await import("./route");
    const response = await PUT(createMockRequest("PUT", { spec: {} }), createMockContext());

    expect(response.status).toBe(403);
  });
});

describe("DELETE /api/workspaces/[name]/toolregistries/[registryName]", () => {
  beforeEach(() => { vi.resetModules(); });
  afterEach(() => { vi.resetAllMocks(); });

  it("deletes tool registry for user with editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { deleteCrd } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    });
    vi.mocked(deleteCrd).mockResolvedValue(undefined);

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(204);
  });

  it("returns 403 when user lacks editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(403);
  });
});
```

- [ ] **Step 6: Run test to verify PUT/DELETE fail**

Run: `cd dashboard && npx vitest run src/app/api/workspaces/\\[name\\]/toolregistries/\\[registryName\\]/route.test.ts`
Expected: FAIL — `PUT` and `DELETE` are not exported.

- [ ] **Step 7: Export PUT and DELETE from item route**

Replace the full contents of `dashboard/src/app/api/workspaces/[name]/toolregistries/[registryName]/route.ts`:

```typescript
/**
 * API route for a specific workspace-scoped tool registry.
 *
 * GET /api/workspaces/:name/toolregistries/:registryName - Get tool registry details
 * PUT /api/workspaces/:name/toolregistries/:registryName - Update tool registry
 * DELETE /api/workspaces/:name/toolregistries/:registryName - Delete tool registry
 *
 * Protected by workspace access checks.
 */

import { createItemRoutes } from "@/lib/api/crd-route-factory";
import type { ToolRegistry } from "@/lib/data/types";

export const { GET, PUT, DELETE } = createItemRoutes<ToolRegistry>({
  kind: "ToolRegistry",
  plural: "toolregistries",
  resourceLabel: "Tool registry",
  paramKey: "registryName",
  errorLabel: "tool registry",
});
```

- [ ] **Step 8: Run all toolregistry tests**

Run: `cd dashboard && npx vitest run src/app/api/workspaces/\\[name\\]/toolregistries/`
Expected: PASS

- [ ] **Step 9: Commit**

```
git add dashboard/src/app/api/workspaces/\[name\]/toolregistries/
```
```
feat(dashboard): enable ToolRegistry write operations

Export POST, PUT, DELETE from workspace-scoped ToolRegistry routes.
The CRD route factory already generates these handlers — they just
weren't exported. Add tests for all CRUD operations.
```

---

### Task 3: Add AgentPolicy TypeScript types and routes

AgentPolicy CRD already exists in Go (`api/v1alpha1/agentpolicy_types.go`). Add hand-written TypeScript types and dashboard routes.

**Files:**
- Create: `dashboard/src/types/agentpolicy.ts`
- Modify: `dashboard/src/types/generated/index.ts`
- Modify: `dashboard/src/lib/k8s/workspace-route-helpers.ts`
- Create: `dashboard/src/app/api/workspaces/[name]/agentpolicies/route.ts`
- Create: `dashboard/src/app/api/workspaces/[name]/agentpolicies/route.test.ts`
- Create: `dashboard/src/app/api/workspaces/[name]/agentpolicies/[policyName]/route.ts`
- Create: `dashboard/src/app/api/workspaces/[name]/agentpolicies/[policyName]/route.test.ts`

- [ ] **Step 1: Create AgentPolicy TypeScript types**

Create `dashboard/src/types/agentpolicy.ts`:

```typescript
/**
 * AgentPolicy CRD TypeScript types.
 *
 * Matches Go types in api/v1alpha1/agentpolicy_types.go.
 */

export type AgentPolicyMode = "enforce" | "permissive";
export type ToolAccessMode = "allowlist" | "denylist";
export type OnFailureAction = "deny" | "allow";
export type AgentPolicyPhase = "Active" | "Error";

export interface AgentPolicySelector {
  agents?: string[];
}

export interface ClaimMappingEntry {
  claim: string;
  header: string;
}

export interface ClaimMapping {
  forwardClaims?: ClaimMappingEntry[];
}

export interface ToolAccessRule {
  registry: string;
  tools: string[];
}

export interface ToolAccessConfig {
  mode: ToolAccessMode;
  rules: ToolAccessRule[];
}

export interface AgentPolicySpec {
  selector?: AgentPolicySelector;
  claimMapping?: ClaimMapping;
  toolAccess?: ToolAccessConfig;
  mode?: AgentPolicyMode;
  onFailure?: OnFailureAction;
}

export interface AgentPolicyCondition {
  type: string;
  status: "True" | "False" | "Unknown";
  lastTransitionTime?: string;
  reason?: string;
  message?: string;
}

export interface AgentPolicyStatus {
  phase?: AgentPolicyPhase;
  matchedAgents?: number;
  conditions?: AgentPolicyCondition[];
  observedGeneration?: number;
}

export interface AgentPolicy {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    namespace?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    uid?: string;
    resourceVersion?: string;
    creationTimestamp?: string;
  };
  spec: AgentPolicySpec;
  status?: AgentPolicyStatus;
}
```

- [ ] **Step 2: Add CRD constant and re-export**

In `dashboard/src/lib/k8s/workspace-route-helpers.ts`, add to the CRD plural constants block:

```typescript
export const CRD_AGENT_POLICIES = "agentpolicies";
```

In `dashboard/src/types/generated/index.ts`, add the re-export:

```typescript
export * from "../agentpolicy";
```

- [ ] **Step 3: Create collection route**

Create `dashboard/src/app/api/workspaces/[name]/agentpolicies/route.ts`:

```typescript
/**
 * API routes for workspace-scoped agent policies.
 *
 * GET /api/workspaces/:name/agentpolicies - List agent policies
 * POST /api/workspaces/:name/agentpolicies - Create a new agent policy
 *
 * Protected by workspace access checks.
 */

import { createCollectionRoutes } from "@/lib/api/crd-route-factory";
import { CRD_AGENT_POLICIES } from "@/lib/k8s/workspace-route-helpers";
import type { AgentPolicy } from "@/types/agentpolicy";

export const { GET, POST } = createCollectionRoutes<AgentPolicy>({
  kind: "AgentPolicy",
  plural: CRD_AGENT_POLICIES,
  errorLabel: "agent policies",
});
```

- [ ] **Step 4: Create item route**

Create `dashboard/src/app/api/workspaces/[name]/agentpolicies/[policyName]/route.ts`:

```typescript
/**
 * API routes for individual workspace agent policy operations.
 *
 * GET /api/workspaces/:name/agentpolicies/:policyName - Get agent policy
 * PUT /api/workspaces/:name/agentpolicies/:policyName - Update agent policy
 * DELETE /api/workspaces/:name/agentpolicies/:policyName - Delete agent policy
 *
 * Protected by workspace access checks.
 */

import { createItemRoutes } from "@/lib/api/crd-route-factory";
import { CRD_AGENT_POLICIES } from "@/lib/k8s/workspace-route-helpers";
import type { AgentPolicy } from "@/types/agentpolicy";

export const { GET, PUT, DELETE } = createItemRoutes<AgentPolicy>({
  kind: "AgentPolicy",
  plural: CRD_AGENT_POLICIES,
  resourceLabel: "Agent policy",
  paramKey: "policyName",
  errorLabel: "agent policy",
});
```

- [ ] **Step 5: Write collection route tests**

Create `dashboard/src/app/api/workspaces/[name]/agentpolicies/route.test.ts`:

```typescript
/**
 * Tests for workspace-scoped agent policy collection API routes.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

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
  isForbiddenError: vi.fn(),
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
      resourceType: "AgentPolicy",
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

const editorPermissions = { read: true, write: true, delete: true, manageMembers: false };
const viewerPermissions = { read: true, write: false, delete: false, manageMembers: false };
const noPermissions = { read: false, write: false, delete: false, manageMembers: false };

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

function createMockRequest(method: string, body?: unknown): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/agentpolicies";
  const init: { method: string; body?: string; headers?: Record<string, string> } = { method };
  if (body) {
    init.body = JSON.stringify(body);
    init.headers = { "Content-Type": "application/json" };
  }
  return new NextRequest(url, init);
}

function createMockContext() {
  return { params: Promise.resolve({ name: "test-ws" }) };
}

describe("GET /api/workspaces/[name]/agentpolicies", () => {
  beforeEach(() => { vi.resetModules(); });
  afterEach(() => { vi.resetAllMocks(); });

  it("returns agent policies for authenticated user with access", async () => {
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

    const mockPolicies = [
      {
        metadata: { name: "default-policy", namespace: "test-ns" },
        spec: { toolAccess: { mode: "allowlist", rules: [] } },
      },
    ];
    vi.mocked(listCrd).mockResolvedValue(mockPolicies);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toHaveLength(1);
    expect(body[0].metadata.name).toBe("default-policy");
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
});

describe("POST /api/workspaces/[name]/agentpolicies", () => {
  beforeEach(() => { vi.resetModules(); });
  afterEach(() => { vi.resetAllMocks(); });

  it("creates agent policy for user with editor role", async () => {
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

    const createdPolicy = {
      metadata: { name: "strict-policy", namespace: "test-ns" },
      spec: {
        toolAccess: {
          mode: "allowlist",
          rules: [{ registry: "search-tools", tools: ["web-search"] }],
        },
      },
    };
    vi.mocked(createCrd).mockResolvedValue(createdPolicy);

    const body = {
      metadata: { name: "strict-policy" },
      spec: createdPolicy.spec,
    };

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", body), createMockContext());

    expect(response.status).toBe(201);
    const result = await response.json();
    expect(result.metadata.name).toBe("strict-policy");
  });

  it("returns 403 when user lacks editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", { metadata: { name: "x" }, spec: {} }), createMockContext());

    expect(response.status).toBe(403);
  });
});
```

- [ ] **Step 6: Write item route tests**

Create `dashboard/src/app/api/workspaces/[name]/agentpolicies/[policyName]/route.test.ts`:

```typescript
/**
 * Tests for workspace-scoped agent policy item API routes.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/k8s/crd-operations", () => ({
  getCrd: vi.fn(),
  updateCrd: vi.fn(),
  deleteCrd: vi.fn(),
  extractK8sErrorMessage: vi.fn((error: unknown) =>
    error instanceof Error ? error.message : String(error)
  ),
  isForbiddenError: vi.fn(),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/workspace-route-helpers")>();
  return {
    ...actual,
    validateWorkspace: vi.fn(),
    getWorkspaceResource: vi.fn(),
    createAuditContext: vi.fn(() => ({
      workspace: "test-ws",
      namespace: "test-ns",
      user: { username: "testuser" },
      role: "editor",
      resourceType: "AgentPolicy",
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

const editorPermissions = { read: true, write: true, delete: true, manageMembers: false };
const viewerPermissions = { read: true, write: false, delete: false, manageMembers: false };

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

const mockPolicy = {
  metadata: {
    name: "strict-policy",
    namespace: "test-ns",
    labels: { "omnia.altairalabs.ai/workspace": "test-ws" },
  },
  spec: {
    toolAccess: {
      mode: "allowlist",
      rules: [{ registry: "search-tools", tools: ["web-search"] }],
    },
  },
};

function createMockRequest(method: string, body?: unknown): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/agentpolicies/strict-policy";
  const init: { method: string; body?: string; headers?: Record<string, string> } = { method };
  if (body) {
    init.body = JSON.stringify(body);
    init.headers = { "Content-Type": "application/json" };
  }
  return new NextRequest(url, init);
}

function createMockContext() {
  return { params: Promise.resolve({ name: "test-ws", policyName: "strict-policy" }) };
}

describe("GET /api/workspaces/[name]/agentpolicies/[policyName]", () => {
  beforeEach(() => { vi.resetModules(); });
  afterEach(() => { vi.resetAllMocks(); });

  it("returns agent policy for authenticated user", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      resource: mockPolicy,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.metadata.name).toBe("strict-policy");
  });
});

describe("PUT /api/workspaces/[name]/agentpolicies/[policyName]", () => {
  beforeEach(() => { vi.resetModules(); });
  afterEach(() => { vi.resetAllMocks(); });

  it("updates agent policy for user with editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { updateCrd } = await import("@/lib/k8s/crd-operations");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
      resource: mockPolicy,
    });

    const updatedPolicy = {
      ...mockPolicy,
      spec: { toolAccess: { mode: "denylist", rules: [{ registry: "dangerous", tools: ["rm-rf"] }] } },
    };
    vi.mocked(updateCrd).mockResolvedValue(updatedPolicy);

    const { PUT } = await import("./route");
    const response = await PUT(createMockRequest("PUT", { spec: updatedPolicy.spec }), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.spec.toolAccess.mode).toBe("denylist");
  });
});

describe("DELETE /api/workspaces/[name]/agentpolicies/[policyName]", () => {
  beforeEach(() => { vi.resetModules(); });
  afterEach(() => { vi.resetAllMocks(); });

  it("deletes agent policy for user with editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { deleteCrd } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    });
    vi.mocked(deleteCrd).mockResolvedValue(undefined);

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(204);
  });
});
```

- [ ] **Step 7: Run all agent policy tests**

Run: `cd dashboard && npx vitest run src/app/api/workspaces/\\[name\\]/agentpolicies/ src/types/agentpolicy.ts`
Expected: PASS

- [ ] **Step 8: Commit**

```
git add dashboard/src/types/agentpolicy.ts dashboard/src/types/generated/index.ts dashboard/src/lib/k8s/workspace-route-helpers.ts dashboard/src/app/api/workspaces/\[name\]/agentpolicies/
```
```
feat(dashboard): add AgentPolicy CRUD routes

Add TypeScript types and workspace-scoped API routes for the AgentPolicy
CRD. Uses the standard CRD route factory pattern with full CRUD support.
```

---

### Task 4: Add ToolPolicy TypeScript types and routes

ToolPolicy is an EE CRD (`ee/api/v1alpha1/toolpolicy_types.go`) for CEL-based parameter validation. Same pattern as Task 3.

**Files:**
- Create: `dashboard/src/types/toolpolicy.ts`
- Modify: `dashboard/src/types/generated/index.ts`
- Modify: `dashboard/src/lib/k8s/workspace-route-helpers.ts`
- Create: `dashboard/src/app/api/workspaces/[name]/toolpolicies/route.ts`
- Create: `dashboard/src/app/api/workspaces/[name]/toolpolicies/route.test.ts`
- Create: `dashboard/src/app/api/workspaces/[name]/toolpolicies/[policyName]/route.ts`
- Create: `dashboard/src/app/api/workspaces/[name]/toolpolicies/[policyName]/route.test.ts`

- [ ] **Step 1: Create ToolPolicy TypeScript types**

Create `dashboard/src/types/toolpolicy.ts`:

```typescript
/**
 * ToolPolicy CRD TypeScript types (Enterprise).
 *
 * Matches Go types in ee/api/v1alpha1/toolpolicy_types.go.
 */

export type PolicyMode = "enforce" | "audit";
export type ToolPolicyOnFailureAction = "deny" | "allow";
export type ToolPolicyPhase = "Active" | "Error";

export interface ToolPolicySelector {
  registry: string;
  tools?: string[];
}

export interface PolicyRuleDeny {
  cel: string;
  message: string;
}

export interface PolicyRule {
  name: string;
  description?: string;
  deny: PolicyRuleDeny;
}

export interface RequiredClaim {
  claim: string;
  message: string;
}

export interface HeaderInjectionRule {
  header: string;
  value?: string;
  cel?: string;
}

export interface ToolPolicyAuditConfig {
  logDecisions?: boolean;
  redactFields?: string[];
}

export interface ToolPolicySpec {
  selector: ToolPolicySelector;
  rules: PolicyRule[];
  requiredClaims?: RequiredClaim[];
  mode?: PolicyMode;
  onFailure?: ToolPolicyOnFailureAction;
  headerInjection?: HeaderInjectionRule[];
  audit?: ToolPolicyAuditConfig;
}

export interface ToolPolicyCondition {
  type: string;
  status: "True" | "False" | "Unknown";
  lastTransitionTime?: string;
  reason?: string;
  message?: string;
}

export interface ToolPolicyStatus {
  phase?: ToolPolicyPhase;
  conditions?: ToolPolicyCondition[];
  observedGeneration?: number;
  ruleCount?: number;
}

export interface ToolPolicy {
  apiVersion: string;
  kind: string;
  metadata: {
    name: string;
    namespace?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    uid?: string;
    resourceVersion?: string;
    creationTimestamp?: string;
  };
  spec: ToolPolicySpec;
  status?: ToolPolicyStatus;
}
```

- [ ] **Step 2: Add CRD constant and re-export**

In `dashboard/src/lib/k8s/workspace-route-helpers.ts`, add to the CRD plural constants block:

```typescript
export const CRD_TOOL_POLICIES = "toolpolicies";
```

In `dashboard/src/types/generated/index.ts`, add the re-export:

```typescript
export * from "../toolpolicy";
```

- [ ] **Step 3: Create collection route**

Create `dashboard/src/app/api/workspaces/[name]/toolpolicies/route.ts`:

```typescript
/**
 * API routes for workspace-scoped tool policies (Enterprise).
 *
 * GET /api/workspaces/:name/toolpolicies - List tool policies
 * POST /api/workspaces/:name/toolpolicies - Create a new tool policy
 *
 * Protected by workspace access checks.
 */

import { createCollectionRoutes } from "@/lib/api/crd-route-factory";
import { CRD_TOOL_POLICIES } from "@/lib/k8s/workspace-route-helpers";
import type { ToolPolicy } from "@/types/toolpolicy";

export const { GET, POST } = createCollectionRoutes<ToolPolicy>({
  kind: "ToolPolicy",
  plural: CRD_TOOL_POLICIES,
  errorLabel: "tool policies",
});
```

- [ ] **Step 4: Create item route**

Create `dashboard/src/app/api/workspaces/[name]/toolpolicies/[policyName]/route.ts`:

```typescript
/**
 * API routes for individual workspace tool policy operations (Enterprise).
 *
 * GET /api/workspaces/:name/toolpolicies/:policyName - Get tool policy
 * PUT /api/workspaces/:name/toolpolicies/:policyName - Update tool policy
 * DELETE /api/workspaces/:name/toolpolicies/:policyName - Delete tool policy
 *
 * Protected by workspace access checks.
 */

import { createItemRoutes } from "@/lib/api/crd-route-factory";
import { CRD_TOOL_POLICIES } from "@/lib/k8s/workspace-route-helpers";
import type { ToolPolicy } from "@/types/toolpolicy";

export const { GET, PUT, DELETE } = createItemRoutes<ToolPolicy>({
  kind: "ToolPolicy",
  plural: CRD_TOOL_POLICIES,
  resourceLabel: "Tool policy",
  paramKey: "policyName",
  errorLabel: "tool policy",
});
```

- [ ] **Step 5: Write collection route tests**

Create `dashboard/src/app/api/workspaces/[name]/toolpolicies/route.test.ts`:

```typescript
/**
 * Tests for workspace-scoped tool policy collection API routes.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

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
  isForbiddenError: vi.fn(),
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
      resourceType: "ToolPolicy",
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

const editorPermissions = { read: true, write: true, delete: true, manageMembers: false };
const viewerPermissions = { read: true, write: false, delete: false, manageMembers: false };
const noPermissions = { read: false, write: false, delete: false, manageMembers: false };

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

function createMockRequest(method: string, body?: unknown): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/toolpolicies";
  const init: { method: string; body?: string; headers?: Record<string, string> } = { method };
  if (body) {
    init.body = JSON.stringify(body);
    init.headers = { "Content-Type": "application/json" };
  }
  return new NextRequest(url, init);
}

function createMockContext() {
  return { params: Promise.resolve({ name: "test-ws" }) };
}

describe("GET /api/workspaces/[name]/toolpolicies", () => {
  beforeEach(() => { vi.resetModules(); });
  afterEach(() => { vi.resetAllMocks(); });

  it("returns tool policies for authenticated user with access", async () => {
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

    const mockPolicies = [
      {
        metadata: { name: "pii-guard", namespace: "test-ns" },
        spec: {
          selector: { registry: "data-tools" },
          rules: [{ name: "no-pii", deny: { cel: "has(params.ssn)", message: "PII not allowed" } }],
        },
      },
    ];
    vi.mocked(listCrd).mockResolvedValue(mockPolicies);

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body).toHaveLength(1);
    expect(body[0].metadata.name).toBe("pii-guard");
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
});

describe("POST /api/workspaces/[name]/toolpolicies", () => {
  beforeEach(() => { vi.resetModules(); });
  afterEach(() => { vi.resetAllMocks(); });

  it("creates tool policy for user with editor role", async () => {
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

    const createdPolicy = {
      metadata: { name: "pii-guard", namespace: "test-ns" },
      spec: {
        selector: { registry: "data-tools" },
        rules: [{ name: "no-pii", deny: { cel: "has(params.ssn)", message: "PII not allowed" } }],
      },
    };
    vi.mocked(createCrd).mockResolvedValue(createdPolicy);

    const { POST } = await import("./route");
    const response = await POST(createMockRequest("POST", {
      metadata: { name: "pii-guard" },
      spec: createdPolicy.spec,
    }), createMockContext());

    expect(response.status).toBe(201);
    const result = await response.json();
    expect(result.metadata.name).toBe("pii-guard");
  });
});
```

- [ ] **Step 6: Write item route tests**

Create `dashboard/src/app/api/workspaces/[name]/toolpolicies/[policyName]/route.test.ts`:

```typescript
/**
 * Tests for workspace-scoped tool policy item API routes.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

vi.mock("@/lib/auth/workspace-authz", () => ({
  checkWorkspaceAccess: vi.fn(),
}));

vi.mock("@/lib/k8s/crd-operations", () => ({
  getCrd: vi.fn(),
  updateCrd: vi.fn(),
  deleteCrd: vi.fn(),
  extractK8sErrorMessage: vi.fn((error: unknown) =>
    error instanceof Error ? error.message : String(error)
  ),
  isForbiddenError: vi.fn(),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/workspace-route-helpers")>();
  return {
    ...actual,
    validateWorkspace: vi.fn(),
    getWorkspaceResource: vi.fn(),
    createAuditContext: vi.fn(() => ({
      workspace: "test-ws",
      namespace: "test-ns",
      user: { username: "testuser" },
      role: "editor",
      resourceType: "ToolPolicy",
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

const editorPermissions = { read: true, write: true, delete: true, manageMembers: false };
const viewerPermissions = { read: true, write: false, delete: false, manageMembers: false };

const mockWorkspace = {
  metadata: { name: "test-ws", namespace: "omnia-system" },
  spec: { namespace: { name: "test-ns" } },
};

const mockPolicy = {
  metadata: {
    name: "pii-guard",
    namespace: "test-ns",
    labels: { "omnia.altairalabs.ai/workspace": "test-ws" },
  },
  spec: {
    selector: { registry: "data-tools" },
    rules: [{ name: "no-pii", deny: { cel: "has(params.ssn)", message: "PII not allowed" } }],
  },
};

function createMockRequest(method: string, body?: unknown): NextRequest {
  const url = "http://localhost:3000/api/workspaces/test-ws/toolpolicies/pii-guard";
  const init: { method: string; body?: string; headers?: Record<string, string> } = { method };
  if (body) {
    init.body = JSON.stringify(body);
    init.headers = { "Content-Type": "application/json" };
  }
  return new NextRequest(url, init);
}

function createMockContext() {
  return { params: Promise.resolve({ name: "test-ws", policyName: "pii-guard" }) };
}

describe("GET /api/workspaces/[name]/toolpolicies/[policyName]", () => {
  beforeEach(() => { vi.resetModules(); });
  afterEach(() => { vi.resetAllMocks(); });

  it("returns tool policy for authenticated user", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "viewer", permissions: viewerPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
      resource: mockPolicy,
    });

    const { GET } = await import("./route");
    const response = await GET(createMockRequest("GET"), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.metadata.name).toBe("pii-guard");
  });
});

describe("PUT /api/workspaces/[name]/toolpolicies/[policyName]", () => {
  beforeEach(() => { vi.resetModules(); });
  afterEach(() => { vi.resetAllMocks(); });

  it("updates tool policy for user with editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { updateCrd } = await import("@/lib/k8s/crd-operations");
    const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
      resource: mockPolicy,
    });

    const updatedPolicy = {
      ...mockPolicy,
      spec: {
        selector: { registry: "data-tools", tools: ["query-db"] },
        rules: [
          { name: "no-pii", deny: { cel: "has(params.ssn)", message: "PII not allowed" } },
          { name: "no-drop", deny: { cel: "params.query.contains('DROP')", message: "DROP not allowed" } },
        ],
      },
    };
    vi.mocked(updateCrd).mockResolvedValue(updatedPolicy);

    const { PUT } = await import("./route");
    const response = await PUT(createMockRequest("PUT", { spec: updatedPolicy.spec }), createMockContext());

    expect(response.status).toBe(200);
    const body = await response.json();
    expect(body.spec.rules).toHaveLength(2);
  });
});

describe("DELETE /api/workspaces/[name]/toolpolicies/[policyName]", () => {
  beforeEach(() => { vi.resetModules(); });
  afterEach(() => { vi.resetAllMocks(); });

  it("deletes tool policy for user with editor role", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { deleteCrd } = await import("@/lib/k8s/crd-operations");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");

    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({ granted: true, role: "editor", permissions: editorPermissions });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: true,
      workspace: mockWorkspace as any,
      clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "editor" },
    });
    vi.mocked(deleteCrd).mockResolvedValue(undefined);

    const { DELETE } = await import("./route");
    const response = await DELETE(createMockRequest("DELETE"), createMockContext());

    expect(response.status).toBe(204);
  });
});
```

- [ ] **Step 7: Run all tool policy tests**

Run: `cd dashboard && npx vitest run src/app/api/workspaces/\\[name\\]/toolpolicies/`
Expected: PASS

- [ ] **Step 8: Commit**

```
git add dashboard/src/types/toolpolicy.ts dashboard/src/types/generated/index.ts dashboard/src/lib/k8s/workspace-route-helpers.ts dashboard/src/app/api/workspaces/\[name\]/toolpolicies/
```
```
feat(dashboard): add ToolPolicy CRUD routes

Add TypeScript types and workspace-scoped API routes for the ToolPolicy
CRD (Enterprise). Uses the standard CRD route factory pattern with full
CRUD support.
```

---

### Task 5: Update OpenAPI spec

Add the new endpoints and schemas to `api/openapi/openapi.yaml`.

**Files:**
- Modify: `api/openapi/openapi.yaml`

- [ ] **Step 1: Add ToolRegistry write endpoints to OpenAPI spec**

In `api/openapi/openapi.yaml`, update the `/api/v1/toolregistries` path to add `post`, and add `put` + `delete` to the item path. Insert after the existing toolregistries GET:

```yaml
  /api/v1/toolregistries:
    get:
      tags: [toolregistries]
      summary: List all ToolRegistries
      operationId: listToolRegistries
      parameters:
        - name: namespace
          in: query
          description: Filter by namespace
          schema:
            type: string
        - name: labelSelector
          in: query
          description: Kubernetes label selector for filtering
          schema:
            type: string
      responses:
        '200':
          description: List of tool registries
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/ToolRegistry'
        '500':
          $ref: '#/components/responses/InternalError'
    post:
      tags: [toolregistries]
      summary: Create a new ToolRegistry
      operationId: createToolRegistry
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ToolRegistry'
      responses:
        '201':
          description: ToolRegistry created successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ToolRegistry'
        '400':
          $ref: '#/components/responses/BadRequest'
        '409':
          description: ToolRegistry already exists
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Error'
        '500':
          $ref: '#/components/responses/InternalError'

  /api/v1/toolregistries/{namespace}/{name}:
    get:
      tags: [toolregistries]
      summary: Get a specific ToolRegistry
      operationId: getToolRegistry
      parameters:
        - name: namespace
          in: path
          required: true
          schema:
            type: string
        - name: name
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: ToolRegistry details
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ToolRegistry'
        '404':
          $ref: '#/components/responses/NotFound'
        '500':
          $ref: '#/components/responses/InternalError'
    put:
      tags: [toolregistries]
      summary: Update a ToolRegistry
      operationId: updateToolRegistry
      parameters:
        - name: namespace
          in: path
          required: true
          schema:
            type: string
        - name: name
          in: path
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ToolRegistry'
      responses:
        '200':
          description: ToolRegistry updated successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ToolRegistry'
        '400':
          $ref: '#/components/responses/BadRequest'
        '404':
          $ref: '#/components/responses/NotFound'
        '500':
          $ref: '#/components/responses/InternalError'
    delete:
      tags: [toolregistries]
      summary: Delete a ToolRegistry
      operationId: deleteToolRegistry
      parameters:
        - name: namespace
          in: path
          required: true
          schema:
            type: string
        - name: name
          in: path
          required: true
          schema:
            type: string
      responses:
        '204':
          description: ToolRegistry deleted successfully
        '404':
          $ref: '#/components/responses/NotFound'
        '500':
          $ref: '#/components/responses/InternalError'
```

- [ ] **Step 2: Add `labelSelector` query param to all list endpoints**

Add the `labelSelector` query parameter to the existing list endpoints for agents, promptpacks, and providers (same format as shown above for toolregistries).

- [ ] **Step 3: Add AgentPolicy and ToolPolicy tags, paths, and schemas**

Add new tags:

```yaml
tags:
  # ... existing tags ...
  - name: agentpolicies
    description: AgentPolicy operations
  - name: toolpolicies
    description: ToolPolicy operations (Enterprise)
```

Add paths (insert before `/api/v1/stats`):

```yaml
  /api/v1/agentpolicies:
    get:
      tags: [agentpolicies]
      summary: List all AgentPolicies
      operationId: listAgentPolicies
      parameters:
        - name: namespace
          in: query
          description: Filter by namespace
          schema:
            type: string
        - name: labelSelector
          in: query
          description: Kubernetes label selector for filtering
          schema:
            type: string
      responses:
        '200':
          description: List of agent policies
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/AgentPolicy'
        '500':
          $ref: '#/components/responses/InternalError'
    post:
      tags: [agentpolicies]
      summary: Create a new AgentPolicy
      operationId: createAgentPolicy
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/AgentPolicy'
      responses:
        '201':
          description: AgentPolicy created successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AgentPolicy'
        '400':
          $ref: '#/components/responses/BadRequest'
        '500':
          $ref: '#/components/responses/InternalError'

  /api/v1/agentpolicies/{namespace}/{name}:
    get:
      tags: [agentpolicies]
      summary: Get a specific AgentPolicy
      operationId: getAgentPolicy
      parameters:
        - name: namespace
          in: path
          required: true
          schema:
            type: string
        - name: name
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: AgentPolicy details
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AgentPolicy'
        '404':
          $ref: '#/components/responses/NotFound'
        '500':
          $ref: '#/components/responses/InternalError'
    put:
      tags: [agentpolicies]
      summary: Update an AgentPolicy
      operationId: updateAgentPolicy
      parameters:
        - name: namespace
          in: path
          required: true
          schema:
            type: string
        - name: name
          in: path
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/AgentPolicy'
      responses:
        '200':
          description: AgentPolicy updated successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/AgentPolicy'
        '404':
          $ref: '#/components/responses/NotFound'
        '500':
          $ref: '#/components/responses/InternalError'
    delete:
      tags: [agentpolicies]
      summary: Delete an AgentPolicy
      operationId: deleteAgentPolicy
      parameters:
        - name: namespace
          in: path
          required: true
          schema:
            type: string
        - name: name
          in: path
          required: true
          schema:
            type: string
      responses:
        '204':
          description: AgentPolicy deleted successfully
        '404':
          $ref: '#/components/responses/NotFound'
        '500':
          $ref: '#/components/responses/InternalError'

  /api/v1/toolpolicies:
    get:
      tags: [toolpolicies]
      summary: List all ToolPolicies
      operationId: listToolPolicies
      parameters:
        - name: namespace
          in: query
          description: Filter by namespace
          schema:
            type: string
        - name: labelSelector
          in: query
          description: Kubernetes label selector for filtering
          schema:
            type: string
      responses:
        '200':
          description: List of tool policies
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/ToolPolicy'
        '500':
          $ref: '#/components/responses/InternalError'
    post:
      tags: [toolpolicies]
      summary: Create a new ToolPolicy
      operationId: createToolPolicy
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ToolPolicy'
      responses:
        '201':
          description: ToolPolicy created successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ToolPolicy'
        '400':
          $ref: '#/components/responses/BadRequest'
        '500':
          $ref: '#/components/responses/InternalError'

  /api/v1/toolpolicies/{namespace}/{name}:
    get:
      tags: [toolpolicies]
      summary: Get a specific ToolPolicy
      operationId: getToolPolicy
      parameters:
        - name: namespace
          in: path
          required: true
          schema:
            type: string
        - name: name
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: ToolPolicy details
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ToolPolicy'
        '404':
          $ref: '#/components/responses/NotFound'
        '500':
          $ref: '#/components/responses/InternalError'
    put:
      tags: [toolpolicies]
      summary: Update a ToolPolicy
      operationId: updateToolPolicy
      parameters:
        - name: namespace
          in: path
          required: true
          schema:
            type: string
        - name: name
          in: path
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/ToolPolicy'
      responses:
        '200':
          description: ToolPolicy updated successfully
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/ToolPolicy'
        '404':
          $ref: '#/components/responses/NotFound'
        '500':
          $ref: '#/components/responses/InternalError'
    delete:
      tags: [toolpolicies]
      summary: Delete a ToolPolicy
      operationId: deleteToolPolicy
      parameters:
        - name: namespace
          in: path
          required: true
          schema:
            type: string
        - name: name
          in: path
          required: true
          schema:
            type: string
      responses:
        '204':
          description: ToolPolicy deleted successfully
        '404':
          $ref: '#/components/responses/NotFound'
        '500':
          $ref: '#/components/responses/InternalError'
```

Add schemas (in the `components.schemas` section):

```yaml
    AgentPolicy:
      type: object
      properties:
        apiVersion:
          type: string
        kind:
          type: string
        metadata:
          $ref: '#/components/schemas/ObjectMeta'
        spec:
          $ref: '#/components/schemas/AgentPolicySpec'
        status:
          $ref: '#/components/schemas/AgentPolicyStatus'

    AgentPolicySpec:
      type: object
      properties:
        selector:
          type: object
          properties:
            agents:
              type: array
              items:
                type: string
        claimMapping:
          type: object
          properties:
            forwardClaims:
              type: array
              items:
                type: object
                required: [claim, header]
                properties:
                  claim:
                    type: string
                  header:
                    type: string
        toolAccess:
          type: object
          properties:
            mode:
              type: string
              enum: [allowlist, denylist]
            rules:
              type: array
              items:
                type: object
                required: [registry, tools]
                properties:
                  registry:
                    type: string
                  tools:
                    type: array
                    items:
                      type: string
        mode:
          type: string
          enum: [enforce, permissive]
          default: enforce
        onFailure:
          type: string
          enum: [deny, allow]
          default: deny

    AgentPolicyStatus:
      type: object
      properties:
        phase:
          type: string
          enum: [Active, Error]
        matchedAgents:
          type: integer
        conditions:
          type: array
          items:
            $ref: '#/components/schemas/Condition'
        observedGeneration:
          type: integer

    ToolPolicy:
      type: object
      properties:
        apiVersion:
          type: string
        kind:
          type: string
        metadata:
          $ref: '#/components/schemas/ObjectMeta'
        spec:
          $ref: '#/components/schemas/ToolPolicySpec'
        status:
          $ref: '#/components/schemas/ToolPolicyStatus'

    ToolPolicySpec:
      type: object
      required: [selector, rules]
      properties:
        selector:
          type: object
          required: [registry]
          properties:
            registry:
              type: string
            tools:
              type: array
              items:
                type: string
        rules:
          type: array
          minItems: 1
          items:
            type: object
            required: [name, deny]
            properties:
              name:
                type: string
              description:
                type: string
              deny:
                type: object
                required: [cel, message]
                properties:
                  cel:
                    type: string
                  message:
                    type: string
        requiredClaims:
          type: array
          items:
            type: object
            required: [claim, message]
            properties:
              claim:
                type: string
              message:
                type: string
        mode:
          type: string
          enum: [enforce, audit]
          default: enforce
        onFailure:
          type: string
          enum: [deny, allow]
          default: deny
        headerInjection:
          type: array
          items:
            type: object
            required: [header]
            properties:
              header:
                type: string
              value:
                type: string
              cel:
                type: string
        audit:
          type: object
          properties:
            logDecisions:
              type: boolean
            redactFields:
              type: array
              items:
                type: string

    ToolPolicyStatus:
      type: object
      properties:
        phase:
          type: string
          enum: [Active, Error]
        conditions:
          type: array
          items:
            $ref: '#/components/schemas/Condition'
        observedGeneration:
          type: integer
        ruleCount:
          type: integer
```

- [ ] **Step 4: Regenerate TypeScript types from OpenAPI spec**

Run: `cd dashboard && npm run generate:api`
Expected: `src/lib/api/schema.d.ts` is regenerated with new types.

- [ ] **Step 5: Commit**

```
git add api/openapi/openapi.yaml dashboard/src/lib/api/schema.d.ts
```
```
docs(api): add AgentPolicy, ToolPolicy, and ToolRegistry write endpoints to OpenAPI spec

Add CRUD endpoints for AgentPolicy and ToolPolicy. Add POST/PUT/DELETE
for ToolRegistry (was read-only). Add labelSelector query param to all
list endpoints.
```

---

### Task 6: Run full dashboard test suite and lint

Final verification that all changes work together.

**Files:** None (verification only)

- [ ] **Step 1: Run all dashboard tests**

Run: `cd dashboard && npx vitest run --coverage`
Expected: PASS with >= 80% coverage on new files.

- [ ] **Step 2: Run TypeScript type check**

Run: `cd dashboard && npm run typecheck`
Expected: PASS — no type errors.

- [ ] **Step 3: Run ESLint**

Run: `cd dashboard && npm run lint`
Expected: PASS — no lint errors.

- [ ] **Step 4: Fix any failures and re-run**

If any tests, type checks, or lint errors occur, fix them and re-run.

- [ ] **Step 5: Final commit if any fixes were needed**

```
fix(dashboard): address test/lint/type issues from deploy adapter API changes
```

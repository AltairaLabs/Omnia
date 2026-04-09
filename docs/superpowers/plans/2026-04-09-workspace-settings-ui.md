# Workspace Settings UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an admin-only workspace settings page with tabs for Overview (status), Services (read-only), and Access (editable anonymous access, role bindings, direct grants).

**Architecture:** New Next.js page at `/workspaces/[name]/settings` with three tab components. A gear icon in the WorkspaceSwitcher provides navigation. A new PATCH API route allows owners to update access settings via K8s merge-patch on the Workspace CRD.

**Tech Stack:** Next.js App Router, React Query, shadcn/ui (Tabs, Card, Table, Badge, Switch, Select), K8s client-node

---

### Task 1: Fix stale WorkspaceStatus TypeScript types

The TS types are out of sync with the Go CRD — missing fields needed by the settings page.

**Files:**
- Modify: `dashboard/src/types/workspace.ts:143-170`

- [ ] **Step 1: Update WorkspaceStatus interface**

In `dashboard/src/types/workspace.ts`, replace the `WorkspaceStatus` interface (lines 143-158) with:

```typescript
/**
 * Workspace status from the Workspace CRD.
 */
export interface WorkspaceStatus {
  /** Current phase of the workspace */
  phase?: "Pending" | "Ready" | "Suspended" | "Error";
  /** Most recent generation observed by the controller */
  observedGeneration?: number;
  /** Namespace status */
  namespace?: {
    name: string;
    created: boolean;
  };
  /** Workspace ServiceAccount names */
  serviceAccounts?: {
    owner: string;
    editor: string;
    viewer: string;
  };
  /** Member counts by role */
  members?: {
    owners: number;
    editors: number;
    viewers: number;
  };
  /** Cost usage tracking */
  costUsage?: CostUsage;
  /** Per-workspace service group statuses */
  services?: ServiceGroupStatus[];
  /** Conditions describing workspace state */
  conditions?: Array<{
    type: string;
    status: "True" | "False" | "Unknown";
    lastTransitionTime?: string;
    reason?: string;
    message?: string;
    observedGeneration?: number;
  }>;
}
```

- [ ] **Step 2: Verify typecheck passes**

Run: `cd dashboard && npm run typecheck`
Expected: no errors (existing code only reads `phase`, `costUsage`, `services`, `conditions` — the new fields are all optional)

- [ ] **Step 3: Commit**

```
git add dashboard/src/types/workspace.ts
```
```
cat <<'EOF' | git commit -F -
fix(dashboard): sync WorkspaceStatus types with Go CRD

Phase values were stale (Active/Terminating vs Pending/Ready/Error/Suspended).
Add missing fields: namespace, serviceAccounts, members, observedGeneration.
EOF
```

---

### Task 2: Add patchWorkspace to K8s client

**Files:**
- Modify: `dashboard/src/lib/k8s/workspace-client.ts`
- Create: `dashboard/src/lib/k8s/workspace-client.test.ts`

- [ ] **Step 1: Write the failing test**

Create `dashboard/src/lib/k8s/workspace-client.test.ts`:

```typescript
import { describe, it, expect, vi, beforeEach } from "vitest";

// Mock the @kubernetes/client-node module
const mockPatchClusterCustomObject = vi.fn();
vi.mock("@kubernetes/client-node", () => {
  const KubeConfig = vi.fn();
  KubeConfig.prototype.loadFromDefault = vi.fn();
  KubeConfig.prototype.makeApiClient = vi.fn(() => ({
    patchClusterCustomObject: mockPatchClusterCustomObject,
  }));
  return { KubeConfig, CustomObjectsApi: vi.fn() };
});

// Must import after mock
const { patchWorkspace } = await import("./workspace-client");

describe("patchWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("sends merge-patch to K8s API", async () => {
    const updatedWorkspace = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "Workspace",
      metadata: { name: "test-ws" },
      spec: { anonymousAccess: { enabled: true, role: "viewer" } },
    };
    mockPatchClusterCustomObject.mockResolvedValue(updatedWorkspace);

    const result = await patchWorkspace("test-ws", {
      anonymousAccess: { enabled: true, role: "viewer" },
    });

    expect(mockPatchClusterCustomObject).toHaveBeenCalledWith(
      expect.objectContaining({
        group: "omnia.altairalabs.ai",
        version: "v1alpha1",
        plural: "workspaces",
        name: "test-ws",
        body: { spec: { anonymousAccess: { enabled: true, role: "viewer" } } },
      })
    );
    expect(result).toEqual(updatedWorkspace);
  });

  it("returns null on error", async () => {
    mockPatchClusterCustomObject.mockRejectedValue(new Error("forbidden"));

    const result = await patchWorkspace("test-ws", {
      anonymousAccess: { enabled: false },
    });

    expect(result).toBeNull();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd dashboard && npx vitest run src/lib/k8s/workspace-client.test.ts`
Expected: FAIL — `patchWorkspace` is not exported

- [ ] **Step 3: Implement patchWorkspace**

In `dashboard/src/lib/k8s/workspace-client.ts`, add after the existing `getWorkspace` function:

```typescript
/**
 * Patch a Workspace CRD using merge-patch.
 * Only sends the fields provided in `updates` — other spec fields are untouched.
 */
export async function patchWorkspace(
  name: string,
  updates: Partial<WorkspaceSpec>
): Promise<Workspace | null> {
  const client = getClient();
  if (!client) return null;

  try {
    const response = await client.patchClusterCustomObject({
      group: GROUP,
      version: VERSION,
      plural: PLURAL,
      name,
      body: { spec: updates },
      headers: { "Content-Type": "application/merge-patch+json" },
    });
    return response as unknown as Workspace;
  } catch (error) {
    console.error(`Failed to patch workspace ${name}:`, error);
    return null;
  }
}
```

Also add the import for `WorkspaceSpec` at the top of the file if not already imported:

```typescript
import type { Workspace, WorkspaceSpec } from "@/types/workspace";
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd dashboard && npx vitest run src/lib/k8s/workspace-client.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add dashboard/src/lib/k8s/workspace-client.ts dashboard/src/lib/k8s/workspace-client.test.ts
```
```
cat <<'EOF' | git commit -F -
feat(dashboard): add patchWorkspace to K8s client

Merge-patches the Workspace CRD spec. Used by the workspace settings
page to update access control fields.
EOF
```

---

### Task 3: Add PATCH API route for workspace updates

**Files:**
- Modify: `dashboard/src/app/api/workspaces/[name]/route.ts`

- [ ] **Step 1: Add PATCH handler**

In `dashboard/src/app/api/workspaces/[name]/route.ts`, add after the existing `GET` export:

```typescript
import { patchWorkspace } from "@/lib/k8s/workspace-client";

/**
 * PATCH /api/workspaces/[name] — update workspace access settings.
 * Owner-only. Accepts partial WorkspaceSpec body with:
 * anonymousAccess, roleBindings, directGrants.
 */
export const PATCH = withWorkspaceAccess(
  "owner",
  async (
    request: NextRequest,
    context: RouteParams,
    _access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    const body = await request.json();

    // Allowlist: only access-related fields can be patched from this route
    const allowed: Partial<WorkspaceSpec> = {};
    if (body.anonymousAccess !== undefined) allowed.anonymousAccess = body.anonymousAccess;
    if (body.roleBindings !== undefined) allowed.roleBindings = body.roleBindings;
    if (body.directGrants !== undefined) allowed.directGrants = body.directGrants;

    if (Object.keys(allowed).length === 0) {
      return NextResponse.json(
        { error: "No updatable fields provided" },
        { status: 400 }
      );
    }

    const updated = await patchWorkspace(name, allowed);
    if (!updated) {
      return NextResponse.json(
        { error: "Failed to update workspace" },
        { status: 500 }
      );
    }

    return NextResponse.json(updated);
  }
);
```

Ensure the existing imports at the top include `WorkspaceSpec`:

```typescript
import type { WorkspaceAccess, WorkspaceSpec } from "@/types/workspace";
```

- [ ] **Step 2: Verify typecheck passes**

Run: `cd dashboard && npm run typecheck`
Expected: PASS

- [ ] **Step 3: Commit**

```
git add dashboard/src/app/api/workspaces/\[name\]/route.ts
```
```
cat <<'EOF' | git commit -F -
feat(dashboard): add PATCH route for workspace access settings

Owner-only endpoint that merge-patches anonymousAccess, roleBindings,
and directGrants on the Workspace CRD.
EOF
```

---

### Task 4: Add useWorkspaceDetail hook

React Query hook for fetching a single workspace's full detail (spec + status) and mutating it.

**Files:**
- Create: `dashboard/src/hooks/use-workspace-detail.ts`
- Create: `dashboard/src/hooks/use-workspace-detail.test.ts`

- [ ] **Step 1: Write the test**

Create `dashboard/src/hooks/use-workspace-detail.test.ts`:

```typescript
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

import { useWorkspaceDetail } from "./use-workspace-detail";

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

describe("useWorkspaceDetail", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("fetches workspace detail", async () => {
    const workspace = {
      metadata: { name: "test-ws" },
      spec: { displayName: "Test" },
      status: { phase: "Ready" },
    };
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(workspace),
    });

    const { result } = renderHook(
      () => useWorkspaceDetail("test-ws"),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.status?.phase).toBe("Ready");
  });

  it("does not fetch when name is null", () => {
    renderHook(
      () => useWorkspaceDetail(null),
      { wrapper: createWrapper() }
    );

    expect(mockFetch).not.toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd dashboard && npx vitest run src/hooks/use-workspace-detail.test.ts`
Expected: FAIL — module not found

- [ ] **Step 3: Implement the hook**

Create `dashboard/src/hooks/use-workspace-detail.ts`:

```typescript
"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import type { Workspace, WorkspaceSpec } from "@/types/workspace";

async function fetchWorkspaceDetail(name: string): Promise<Workspace> {
  const response = await fetch(`/api/workspaces/${encodeURIComponent(name)}`);
  if (!response.ok) {
    throw new Error(`Failed to fetch workspace: ${response.statusText}`);
  }
  return response.json();
}

async function patchWorkspaceAccess(
  name: string,
  updates: Partial<WorkspaceSpec>
): Promise<Workspace> {
  const response = await fetch(`/api/workspaces/${encodeURIComponent(name)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(updates),
  });
  if (!response.ok) {
    throw new Error(`Failed to update workspace: ${response.statusText}`);
  }
  return response.json();
}

/**
 * Fetch full workspace detail (spec + status) for the settings page.
 */
export function useWorkspaceDetail(name: string | null) {
  return useQuery({
    queryKey: ["workspace-detail", name],
    queryFn: () => fetchWorkspaceDetail(name!),
    enabled: !!name,
    staleTime: 30000,
  });
}

/**
 * Mutation hook for patching workspace access settings.
 * Optimistically updates the cache and rolls back on failure.
 */
export function useWorkspacePatch(name: string) {
  const queryClient = useQueryClient();
  const queryKey = ["workspace-detail", name];

  return useMutation({
    mutationFn: (updates: Partial<WorkspaceSpec>) =>
      patchWorkspaceAccess(name, updates),

    onMutate: async (updates) => {
      await queryClient.cancelQueries({ queryKey });
      const previous = queryClient.getQueryData<Workspace>(queryKey);

      queryClient.setQueryData<Workspace>(queryKey, (old) => {
        if (!old) return old;
        return { ...old, spec: { ...old.spec, ...updates } };
      });

      return { previous };
    },

    onError: (_err, _updates, context) => {
      if (context?.previous) {
        queryClient.setQueryData(queryKey, context.previous);
      }
    },

    onSettled: () => {
      queryClient.invalidateQueries({ queryKey });
    },
  });
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd dashboard && npx vitest run src/hooks/use-workspace-detail.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add dashboard/src/hooks/use-workspace-detail.ts dashboard/src/hooks/use-workspace-detail.test.ts
```
```
cat <<'EOF' | git commit -F -
feat(dashboard): add useWorkspaceDetail and useWorkspacePatch hooks

React Query hooks for fetching full workspace CRD data and patching
access settings with optimistic updates.
EOF
```

---

### Task 5: Build Overview tab component

**Files:**
- Create: `dashboard/src/app/workspaces/[name]/settings/overview-tab.tsx`
- Create: `dashboard/src/app/workspaces/[name]/settings/overview-tab.test.tsx`

- [ ] **Step 1: Write the test**

Create `dashboard/src/app/workspaces/[name]/settings/overview-tab.test.tsx`:

```typescript
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { OverviewTab } from "./overview-tab";
import type { Workspace } from "@/types/workspace";

const workspace: Workspace = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "Workspace",
  metadata: { name: "test-ws", creationTimestamp: "2026-01-01T00:00:00Z" },
  spec: {
    displayName: "Test Workspace",
    description: "A test workspace",
    environment: "development",
    namespace: { name: "test-ns" },
  },
  status: {
    phase: "Ready",
    observedGeneration: 3,
    namespace: { name: "test-ns", created: true },
    serviceAccounts: {
      owner: "ws-test-owner-sa",
      editor: "ws-test-editor-sa",
      viewer: "ws-test-viewer-sa",
    },
    conditions: [
      {
        type: "NamespaceReady",
        status: "True",
        reason: "NamespaceReady",
        message: "Namespace is ready",
        lastTransitionTime: "2026-01-01T00:00:00Z",
      },
      {
        type: "RoleBindingsReady",
        status: "False",
        reason: "RoleBindingsFailed",
        message: "RBAC error details here",
        lastTransitionTime: "2026-01-01T00:01:00Z",
      },
    ],
  },
};

describe("OverviewTab", () => {
  it("renders workspace phase badge", () => {
    render(<OverviewTab workspace={workspace} />);
    expect(screen.getByText("Ready")).toBeInTheDocument();
  });

  it("renders workspace details", () => {
    render(<OverviewTab workspace={workspace} />);
    expect(screen.getByText("Test Workspace")).toBeInTheDocument();
    expect(screen.getByText("development")).toBeInTheDocument();
    expect(screen.getByText("test-ns")).toBeInTheDocument();
  });

  it("renders service accounts", () => {
    render(<OverviewTab workspace={workspace} />);
    expect(screen.getByText("ws-test-owner-sa")).toBeInTheDocument();
    expect(screen.getByText("ws-test-editor-sa")).toBeInTheDocument();
    expect(screen.getByText("ws-test-viewer-sa")).toBeInTheDocument();
  });

  it("highlights error conditions", () => {
    render(<OverviewTab workspace={workspace} />);
    expect(screen.getByText("RoleBindingsFailed")).toBeInTheDocument();
    expect(screen.getByText("RBAC error details here")).toBeInTheDocument();
  });

  it("handles missing status gracefully", () => {
    const minimal: Workspace = {
      ...workspace,
      status: undefined,
    };
    render(<OverviewTab workspace={minimal} />);
    expect(screen.getByText("Pending")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd dashboard && npx vitest run src/app/workspaces/\\[name\\]/settings/overview-tab.test.tsx`
Expected: FAIL — module not found

- [ ] **Step 3: Implement OverviewTab**

Create `dashboard/src/app/workspaces/[name]/settings/overview-tab.tsx`:

```tsx
"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { Workspace } from "@/types/workspace";

interface OverviewTabProps {
  workspace: Workspace;
}

const PHASE_VARIANT: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  Ready: "default",
  Pending: "secondary",
  Error: "destructive",
  Suspended: "outline",
};

function DetailRow({ label, value }: { label: string; value: string | undefined }) {
  return (
    <div className="flex justify-between py-2 border-b last:border-0">
      <span className="text-sm text-muted-foreground">{label}</span>
      <span className="text-sm font-medium">{value ?? "—"}</span>
    </div>
  );
}

export function OverviewTab({ workspace }: OverviewTabProps) {
  const { spec, status, metadata } = workspace;
  const phase = status?.phase ?? "Pending";
  const conditions = status?.conditions ?? [];
  const sa = status?.serviceAccounts;

  return (
    <div className="space-y-6">
      {/* Status + Details */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between pb-2">
          <CardTitle className="text-base">Details</CardTitle>
          <Badge variant={PHASE_VARIANT[phase] ?? "secondary"}>{phase}</Badge>
        </CardHeader>
        <CardContent>
          <DetailRow label="Display Name" value={spec.displayName} />
          <DetailRow label="Description" value={spec.description} />
          <DetailRow label="Environment" value={spec.environment} />
          <DetailRow label="Namespace" value={status?.namespace?.name ?? spec.namespace?.name} />
          <DetailRow label="Created" value={metadata.creationTimestamp} />
          <DetailRow
            label="Observed Generation"
            value={status?.observedGeneration?.toString()}
          />
        </CardContent>
      </Card>

      {/* Service Accounts */}
      {sa && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Service Accounts</CardTitle>
          </CardHeader>
          <CardContent>
            <DetailRow label="Owner" value={sa.owner} />
            <DetailRow label="Editor" value={sa.editor} />
            <DetailRow label="Viewer" value={sa.viewer} />
          </CardContent>
        </Card>
      )}

      {/* Conditions */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Conditions</CardTitle>
        </CardHeader>
        <CardContent>
          {conditions.length === 0 ? (
            <p className="text-sm text-muted-foreground">No conditions reported</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Type</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Reason</TableHead>
                  <TableHead>Message</TableHead>
                  <TableHead>Last Transition</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {conditions.map((c) => (
                  <TableRow
                    key={c.type}
                    className={c.status === "False" ? "bg-destructive/5" : ""}
                  >
                    <TableCell className="font-medium">{c.type}</TableCell>
                    <TableCell>
                      <Badge
                        variant={c.status === "True" ? "default" : "destructive"}
                      >
                        {c.status}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-sm">{c.reason}</TableCell>
                    <TableCell className="text-sm max-w-md truncate">
                      {c.message}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {c.lastTransitionTime}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd dashboard && npx vitest run src/app/workspaces/\\[name\\]/settings/overview-tab.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add dashboard/src/app/workspaces/\[name\]/settings/overview-tab.tsx dashboard/src/app/workspaces/\[name\]/settings/overview-tab.test.tsx
```
```
cat <<'EOF' | git commit -F -
feat(dashboard): add workspace settings Overview tab

Read-only status view with phase badge, details grid, service accounts,
and conditions table with error highlighting.
EOF
```

---

### Task 6: Build Services tab component

**Files:**
- Create: `dashboard/src/app/workspaces/[name]/settings/services-tab.tsx`
- Create: `dashboard/src/app/workspaces/[name]/settings/services-tab.test.tsx`

- [ ] **Step 1: Write the test**

Create `dashboard/src/app/workspaces/[name]/settings/services-tab.test.tsx`:

```typescript
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ServicesTab } from "./services-tab";
import type { Workspace } from "@/types/workspace";

const workspace: Workspace = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "Workspace",
  metadata: { name: "test-ws" },
  spec: {
    displayName: "Test",
    environment: "development",
    namespace: { name: "test-ns" },
    services: [
      {
        name: "default",
        mode: "managed",
        session: { database: { secretRef: { name: "pg-secret" } } },
        memory: { database: { secretRef: { name: "pg-secret" } } },
      },
    ],
  },
  status: {
    phase: "Ready",
    services: [
      {
        name: "default",
        sessionURL: "http://session-test-ns-default:8080",
        memoryURL: "http://memory-test-ns-default:8080",
        ready: true,
      },
    ],
  },
};

describe("ServicesTab", () => {
  it("renders service group name and mode", () => {
    render(<ServicesTab workspace={workspace} />);
    expect(screen.getByText("default")).toBeInTheDocument();
    expect(screen.getByText("managed")).toBeInTheDocument();
  });

  it("renders session and memory URLs", () => {
    render(<ServicesTab workspace={workspace} />);
    expect(
      screen.getByText("http://session-test-ns-default:8080")
    ).toBeInTheDocument();
    expect(
      screen.getByText("http://memory-test-ns-default:8080")
    ).toBeInTheDocument();
  });

  it("shows ready indicator", () => {
    render(<ServicesTab workspace={workspace} />);
    // Two ready indicators (session + memory)
    const readyDots = screen.getAllByTestId("status-ready");
    expect(readyDots.length).toBeGreaterThanOrEqual(1);
  });

  it("shows provisioning message when no services in status", () => {
    const pending: Workspace = {
      ...workspace,
      status: { phase: "Pending" },
    };
    render(<ServicesTab workspace={pending} />);
    expect(screen.getByText(/being provisioned/i)).toBeInTheDocument();
  });

  it("shows notice when no service groups configured", () => {
    const noServices: Workspace = {
      ...workspace,
      spec: { ...workspace.spec, services: undefined },
    };
    render(<ServicesTab workspace={noServices} />);
    expect(screen.getByText(/no service groups/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd dashboard && npx vitest run src/app/workspaces/\\[name\\]/settings/services-tab.test.tsx`
Expected: FAIL — module not found

- [ ] **Step 3: Implement ServicesTab**

Create `dashboard/src/app/workspaces/[name]/settings/services-tab.tsx`:

```tsx
"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Copy, Info } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { Workspace } from "@/types/workspace";

interface ServicesTabProps {
  workspace: Workspace;
}

function StatusDot({ ready }: { ready: boolean }) {
  return (
    <span
      data-testid={ready ? "status-ready" : "status-not-ready"}
      className={`inline-block h-2.5 w-2.5 rounded-full ${ready ? "bg-green-500" : "bg-red-500"}`}
    />
  );
}

function CopyButton({ text }: { text: string }) {
  return (
    <Button
      variant="ghost"
      size="icon"
      className="h-6 w-6"
      onClick={() => navigator.clipboard.writeText(text)}
    >
      <Copy className="h-3 w-3" />
    </Button>
  );
}

function ServiceCard({
  label,
  url,
  ready,
  secretRef,
}: {
  label: string;
  url?: string;
  ready: boolean;
  secretRef?: string;
}) {
  return (
    <div className="flex flex-col gap-1 rounded-md border p-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">{label}</span>
        <StatusDot ready={ready} />
      </div>
      {url ? (
        <div className="flex items-center gap-1">
          <code className="text-xs text-muted-foreground break-all">{url}</code>
          <CopyButton text={url} />
        </div>
      ) : (
        <span className="text-xs text-muted-foreground">Not available</span>
      )}
      {secretRef && (
        <span className="text-xs text-muted-foreground">
          Secret: {secretRef}
        </span>
      )}
    </div>
  );
}

export function ServicesTab({ workspace }: ServicesTabProps) {
  const specServices = workspace.spec.services;
  const statusServices = workspace.status?.services ?? [];

  if (!specServices || specServices.length === 0) {
    return (
      <Alert>
        <Info className="h-4 w-4" />
        <AlertTitle>No service groups configured</AlertTitle>
        <AlertDescription>
          Add a services block to the Workspace CRD to provision session-api and
          memory-api instances.
        </AlertDescription>
      </Alert>
    );
  }

  if (statusServices.length === 0) {
    return (
      <Alert>
        <Info className="h-4 w-4" />
        <AlertTitle>Services being provisioned</AlertTitle>
        <AlertDescription>
          The operator is provisioning services for this workspace. This may take
          a minute.
        </AlertDescription>
      </Alert>
    );
  }

  return (
    <div className="space-y-4">
      {specServices.map((sg) => {
        const status = statusServices.find((s) => s.name === sg.name);
        return (
          <Card key={sg.name}>
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-base">{sg.name}</CardTitle>
              <Badge variant="outline">{sg.mode}</Badge>
            </CardHeader>
            <CardContent className="grid gap-3 sm:grid-cols-2">
              <ServiceCard
                label="Session API"
                url={status?.sessionURL}
                ready={!!status?.ready}
                secretRef={sg.session?.database?.secretRef?.name}
              />
              <ServiceCard
                label="Memory API"
                url={status?.memoryURL}
                ready={!!status?.ready}
                secretRef={sg.memory?.database?.secretRef?.name}
              />
            </CardContent>
          </Card>
        );
      })}
    </div>
  );
}
```

Note: The `spec.services` field uses the `WorkspaceServiceGroup` type. Verify the TS types include `services` on `WorkspaceSpec` — if not, add it:

```typescript
// In dashboard/src/types/workspace.ts, inside WorkspaceSpec:
services?: Array<{
  name: string;
  mode: "managed" | "external";
  session?: { database?: { secretRef?: { name: string } } };
  memory?: { database?: { secretRef?: { name: string } } };
}>;
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd dashboard && npx vitest run src/app/workspaces/\\[name\\]/settings/services-tab.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add dashboard/src/app/workspaces/\[name\]/settings/services-tab.tsx dashboard/src/app/workspaces/\[name\]/settings/services-tab.test.tsx
```
```
cat <<'EOF' | git commit -F -
feat(dashboard): add workspace settings Services tab

Read-only view of per-workspace service groups with URLs, readiness
indicators, and database secret refs.
EOF
```

---

### Task 7: Build Access tab component

**Files:**
- Create: `dashboard/src/app/workspaces/[name]/settings/access-tab.tsx`
- Create: `dashboard/src/app/workspaces/[name]/settings/access-tab.test.tsx`

- [ ] **Step 1: Write the test**

Create `dashboard/src/app/workspaces/[name]/settings/access-tab.test.tsx`:

```typescript
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AccessTab } from "./access-tab";
import type { Workspace } from "@/types/workspace";

const mockMutate = vi.fn();

const workspace: Workspace = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "Workspace",
  metadata: { name: "test-ws" },
  spec: {
    displayName: "Test",
    environment: "development",
    namespace: { name: "test-ns" },
    anonymousAccess: { enabled: true, role: "viewer" },
    roleBindings: [
      { groups: ["dev-team"], role: "editor" },
      { groups: ["ops-team"], role: "owner" },
    ],
    directGrants: [
      { user: "alice@example.com", role: "owner" },
      { user: "bob@example.com", role: "viewer", expires: "2026-12-31T00:00:00Z" },
    ],
  },
};

describe("AccessTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders anonymous access toggle as enabled", () => {
    render(<AccessTab workspace={workspace} onPatch={mockMutate} />);
    const toggle = screen.getByRole("switch");
    expect(toggle).toBeChecked();
  });

  it("renders role bindings table", () => {
    render(<AccessTab workspace={workspace} onPatch={mockMutate} />);
    expect(screen.getByText("dev-team")).toBeInTheDocument();
    expect(screen.getByText("ops-team")).toBeInTheDocument();
  });

  it("renders direct grants table", () => {
    render(<AccessTab workspace={workspace} onPatch={mockMutate} />);
    expect(screen.getByText("alice@example.com")).toBeInTheDocument();
    expect(screen.getByText("bob@example.com")).toBeInTheDocument();
  });

  it("calls onPatch when anonymous access is toggled off", async () => {
    const user = userEvent.setup();
    render(<AccessTab workspace={workspace} onPatch={mockMutate} />);
    await user.click(screen.getByRole("switch"));
    expect(mockMutate).toHaveBeenCalledWith({
      anonymousAccess: { enabled: false },
    });
  });

  it("calls onPatch when a role binding is deleted", async () => {
    const user = userEvent.setup();
    render(<AccessTab workspace={workspace} onPatch={mockMutate} />);
    const deleteButtons = screen.getAllByTestId("delete-binding");
    await user.click(deleteButtons[0]);
    expect(mockMutate).toHaveBeenCalledWith({
      roleBindings: [{ groups: ["ops-team"], role: "owner" }],
    });
  });

  it("calls onPatch when a direct grant is deleted", async () => {
    const user = userEvent.setup();
    render(<AccessTab workspace={workspace} onPatch={mockMutate} />);
    const deleteButtons = screen.getAllByTestId("delete-grant");
    await user.click(deleteButtons[0]);
    expect(mockMutate).toHaveBeenCalledWith({
      directGrants: [
        { user: "bob@example.com", role: "viewer", expires: "2026-12-31T00:00:00Z" },
      ],
    });
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd dashboard && npx vitest run src/app/workspaces/\\[name\\]/settings/access-tab.test.tsx`
Expected: FAIL — module not found

- [ ] **Step 3: Implement AccessTab**

Create `dashboard/src/app/workspaces/[name]/settings/access-tab.tsx`:

```tsx
"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { AlertTriangle, Plus, Trash2 } from "lucide-react";
import type { Workspace, WorkspaceRole, WorkspaceSpec } from "@/types/workspace";

interface AccessTabProps {
  workspace: Workspace;
  onPatch: (updates: Partial<WorkspaceSpec>) => void;
}

const ROLES: WorkspaceRole[] = ["viewer", "editor", "owner"];

export function AccessTab({ workspace, onPatch }: AccessTabProps) {
  const { spec } = workspace;
  const anonymousAccess = spec.anonymousAccess ?? { enabled: false };
  const roleBindings = spec.roleBindings ?? [];
  const directGrants = spec.directGrants ?? [];

  // Inline add state
  const [newBindingGroup, setNewBindingGroup] = useState("");
  const [newBindingRole, setNewBindingRole] = useState<WorkspaceRole>("viewer");
  const [newGrantUser, setNewGrantUser] = useState("");
  const [newGrantRole, setNewGrantRole] = useState<WorkspaceRole>("viewer");

  const handleToggleAnonymous = () => {
    onPatch({
      anonymousAccess: { enabled: !anonymousAccess.enabled },
    });
  };

  const handleAnonymousRoleChange = (role: WorkspaceRole) => {
    onPatch({
      anonymousAccess: { ...anonymousAccess, role },
    });
  };

  const handleDeleteBinding = (index: number) => {
    onPatch({
      roleBindings: roleBindings.filter((_, i) => i !== index),
    });
  };

  const handleAddBinding = () => {
    if (!newBindingGroup.trim()) return;
    onPatch({
      roleBindings: [
        ...roleBindings,
        { groups: [newBindingGroup.trim()], role: newBindingRole },
      ],
    });
    setNewBindingGroup("");
    setNewBindingRole("viewer");
  };

  const handleDeleteGrant = (index: number) => {
    onPatch({
      directGrants: directGrants.filter((_, i) => i !== index),
    });
  };

  const handleAddGrant = () => {
    if (!newGrantUser.trim()) return;
    onPatch({
      directGrants: [
        ...directGrants,
        { user: newGrantUser.trim(), role: newGrantRole },
      ],
    });
    setNewGrantUser("");
    setNewGrantRole("viewer");
  };

  return (
    <div className="space-y-6">
      {/* Anonymous Access */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Anonymous Access</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex items-center justify-between">
            <span className="text-sm">Allow unauthenticated users</span>
            <Switch
              checked={anonymousAccess.enabled}
              onCheckedChange={handleToggleAnonymous}
            />
          </div>
          {anonymousAccess.enabled && (
            <div className="flex items-center gap-3">
              <span className="text-sm text-muted-foreground">Role:</span>
              <Select
                value={anonymousAccess.role ?? "viewer"}
                onValueChange={(v) => handleAnonymousRoleChange(v as WorkspaceRole)}
              >
                <SelectTrigger className="w-32">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {ROLES.map((r) => (
                    <SelectItem key={r} value={r}>
                      {r}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {anonymousAccess.role && anonymousAccess.role !== "viewer" && (
                <Badge variant="outline" className="text-yellow-600 border-yellow-600">
                  <AlertTriangle className="h-3 w-3 mr-1" />
                  Elevated access
                </Badge>
              )}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Role Bindings */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between pb-2">
          <CardTitle className="text-base">Role Bindings</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Group</TableHead>
                <TableHead>Role</TableHead>
                <TableHead className="w-16" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {roleBindings.map((rb, i) => (
                <TableRow key={`${rb.groups?.join(",")}-${rb.role}`}>
                  <TableCell>{rb.groups?.join(", ")}</TableCell>
                  <TableCell>
                    <Badge variant="outline">{rb.role}</Badge>
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="icon"
                      data-testid="delete-binding"
                      onClick={() => handleDeleteBinding(i)}
                    >
                      <Trash2 className="h-4 w-4 text-destructive" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
              {/* Inline add row */}
              <TableRow>
                <TableCell>
                  <Input
                    placeholder="Group name"
                    value={newBindingGroup}
                    onChange={(e) => setNewBindingGroup(e.target.value)}
                    onKeyDown={(e) => e.key === "Enter" && handleAddBinding()}
                    className="h-8"
                  />
                </TableCell>
                <TableCell>
                  <Select
                    value={newBindingRole}
                    onValueChange={(v) => setNewBindingRole(v as WorkspaceRole)}
                  >
                    <SelectTrigger className="h-8 w-28">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {ROLES.map((r) => (
                        <SelectItem key={r} value={r}>
                          {r}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </TableCell>
                <TableCell>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={handleAddBinding}
                    disabled={!newBindingGroup.trim()}
                  >
                    <Plus className="h-4 w-4" />
                  </Button>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {/* Direct Grants */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between pb-2">
          <CardTitle className="text-base">Direct Grants</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>User</TableHead>
                <TableHead>Role</TableHead>
                <TableHead>Expires</TableHead>
                <TableHead className="w-16" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {directGrants.map((grant, i) => (
                <TableRow key={`${grant.user}-${grant.role}`}>
                  <TableCell>{grant.user}</TableCell>
                  <TableCell>
                    <Badge variant="outline">{grant.role}</Badge>
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {grant.expires ?? "Never"}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="icon"
                      data-testid="delete-grant"
                      onClick={() => handleDeleteGrant(i)}
                    >
                      <Trash2 className="h-4 w-4 text-destructive" />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
              {/* Inline add row */}
              <TableRow>
                <TableCell>
                  <Input
                    placeholder="User email"
                    value={newGrantUser}
                    onChange={(e) => setNewGrantUser(e.target.value)}
                    onKeyDown={(e) => e.key === "Enter" && handleAddGrant()}
                    className="h-8"
                  />
                </TableCell>
                <TableCell>
                  <Select
                    value={newGrantRole}
                    onValueChange={(v) => setNewGrantRole(v as WorkspaceRole)}
                  >
                    <SelectTrigger className="h-8 w-28">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {ROLES.map((r) => (
                        <SelectItem key={r} value={r}>
                          {r}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </TableCell>
                <TableCell />
                <TableCell>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={handleAddGrant}
                    disabled={!newGrantUser.trim()}
                  >
                    <Plus className="h-4 w-4" />
                  </Button>
                </TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd dashboard && npx vitest run src/app/workspaces/\\[name\\]/settings/access-tab.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add dashboard/src/app/workspaces/\[name\]/settings/access-tab.tsx dashboard/src/app/workspaces/\[name\]/settings/access-tab.test.tsx
```
```
cat <<'EOF' | git commit -F -
feat(dashboard): add workspace settings Access tab

Editable anonymous access toggle, role bindings table, and direct grants
table with inline add/delete and optimistic PATCH updates.
EOF
```

---

### Task 8: Build main settings page with tabs

**Files:**
- Create: `dashboard/src/app/workspaces/[name]/settings/page.tsx`
- Create: `dashboard/src/app/workspaces/[name]/settings/page.test.tsx`

- [ ] **Step 1: Write the test**

Create `dashboard/src/app/workspaces/[name]/settings/page.test.tsx`:

```typescript
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const mockUseWorkspaceDetail = vi.fn();
const mockUseWorkspacePatch = vi.fn();
const mockUseParams = vi.fn();

vi.mock("@/hooks/use-workspace-detail", () => ({
  useWorkspaceDetail: () => mockUseWorkspaceDetail(),
  useWorkspacePatch: () => mockUseWorkspacePatch(),
}));

vi.mock("next/navigation", () => ({
  useParams: () => mockUseParams(),
}));

vi.mock("@/components/layout", () => ({
  Header: ({ title, description }: { title: string; description?: string }) => (
    <div data-testid="header">
      <h1>{title}</h1>
      {description && <p>{description}</p>}
    </div>
  ),
}));

import WorkspaceSettingsPage from "./page";

const workspace = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1" as const,
  kind: "Workspace" as const,
  metadata: { name: "test-ws" },
  spec: {
    displayName: "Test Workspace",
    environment: "development" as const,
    namespace: { name: "test-ns" },
  },
  status: { phase: "Ready" as const },
};

describe("WorkspaceSettingsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseParams.mockReturnValue({ name: "test-ws" });
    mockUseWorkspaceDetail.mockReturnValue({
      data: workspace,
      isLoading: false,
      error: null,
    });
    mockUseWorkspacePatch.mockReturnValue({ mutate: vi.fn() });
  });

  it("renders the header", () => {
    render(<WorkspaceSettingsPage />);
    expect(screen.getByText("Workspace Settings")).toBeInTheDocument();
  });

  it("renders three tabs", () => {
    render(<WorkspaceSettingsPage />);
    expect(screen.getByText("Overview")).toBeInTheDocument();
    expect(screen.getByText("Services")).toBeInTheDocument();
    expect(screen.getByText("Access")).toBeInTheDocument();
  });

  it("shows loading skeleton while fetching", () => {
    mockUseWorkspaceDetail.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    });
    render(<WorkspaceSettingsPage />);
    expect(screen.getByTestId("settings-loading")).toBeInTheDocument();
  });

  it("shows error alert on fetch failure", () => {
    mockUseWorkspaceDetail.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("not found"),
    });
    render(<WorkspaceSettingsPage />);
    expect(screen.getByText(/not found/i)).toBeInTheDocument();
  });

  it("switches to Services tab on click", async () => {
    const user = userEvent.setup();
    render(<WorkspaceSettingsPage />);
    await user.click(screen.getByText("Services"));
    // Services tab should show the "no service groups" message since spec has none
    expect(screen.getByText(/no service groups/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd dashboard && npx vitest run src/app/workspaces/\\[name\\]/settings/page.test.tsx`
Expected: FAIL — module not found

- [ ] **Step 3: Implement the page**

Create `dashboard/src/app/workspaces/[name]/settings/page.tsx`:

```tsx
"use client";

import { useParams } from "next/navigation";
import { Header } from "@/components/layout";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { AlertCircle } from "lucide-react";
import { useWorkspaceDetail, useWorkspacePatch } from "@/hooks/use-workspace-detail";
import { OverviewTab } from "./overview-tab";
import { ServicesTab } from "./services-tab";
import { AccessTab } from "./access-tab";

export default function WorkspaceSettingsPage() {
  const params = useParams<{ name: string }>();
  const name = params.name;
  const { data: workspace, isLoading, error } = useWorkspaceDetail(name);
  const { mutate } = useWorkspacePatch(name);

  if (isLoading) {
    return (
      <div className="flex flex-col h-full" data-testid="settings-loading">
        <Header title="Workspace Settings" />
        <div className="p-6 space-y-4">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      </div>
    );
  }

  if (error || !workspace) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Workspace Settings" />
        <div className="p-6">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Failed to load workspace</AlertTitle>
            <AlertDescription>
              {error instanceof Error ? error.message : "Workspace not found"}
            </AlertDescription>
          </Alert>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <Header
        title="Workspace Settings"
        description={workspace.spec.displayName}
      />

      <div className="flex-1 overflow-auto p-6">
        <Tabs defaultValue="overview">
          <TabsList>
            <TabsTrigger value="overview">Overview</TabsTrigger>
            <TabsTrigger value="services">Services</TabsTrigger>
            <TabsTrigger value="access">Access</TabsTrigger>
          </TabsList>

          <TabsContent value="overview" className="mt-4">
            <OverviewTab workspace={workspace} />
          </TabsContent>

          <TabsContent value="services" className="mt-4">
            <ServicesTab workspace={workspace} />
          </TabsContent>

          <TabsContent value="access" className="mt-4">
            <AccessTab workspace={workspace} onPatch={mutate} />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd dashboard && npx vitest run src/app/workspaces/\\[name\\]/settings/page.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```
git add dashboard/src/app/workspaces/\[name\]/settings/page.tsx dashboard/src/app/workspaces/\[name\]/settings/page.test.tsx
```
```
cat <<'EOF' | git commit -F -
feat(dashboard): add workspace settings page with tabs

Main page at /workspaces/[name]/settings with Overview, Services, and
Access tabs. Uses useWorkspaceDetail for data and useWorkspacePatch for
optimistic access control updates.
EOF
```

---

### Task 9: Add gear icon to WorkspaceSwitcher

**Files:**
- Modify: `dashboard/src/components/workspace-switcher.tsx`

- [ ] **Step 1: Add gear icon with navigation**

In `dashboard/src/components/workspace-switcher.tsx`:

1. Add imports at the top:
```typescript
import { Settings } from "lucide-react";
import { useRouter } from "next/navigation";
import { useWorkspacePermissions } from "@/hooks/use-workspace-permissions";
```

2. Inside the component, add:
```typescript
const router = useRouter();
const { isOwner } = useWorkspacePermissions();
```

3. In the JSX, between the role Badge and the ChevronsUpDown icon (around line 104), add:
```tsx
{isOwner && currentWorkspace && (
  <Button
    variant="ghost"
    size="icon"
    className="h-6 w-6"
    data-testid="workspace-settings-gear"
    onClick={(e) => {
      e.stopPropagation();
      router.push(`/workspaces/${currentWorkspace.name}/settings`);
    }}
  >
    <Settings className="h-3.5 w-3.5 opacity-50 hover:opacity-100" />
  </Button>
)}
```

The `e.stopPropagation()` prevents the click from toggling the workspace dropdown.

- [ ] **Step 2: Verify typecheck passes**

Run: `cd dashboard && npm run typecheck`
Expected: PASS

- [ ] **Step 3: Verify the build passes**

Run: `cd dashboard && npx next build`
Expected: PASS (no build errors)

- [ ] **Step 4: Commit**

```
git add dashboard/src/components/workspace-switcher.tsx
```
```
cat <<'EOF' | git commit -F -
feat(dashboard): add gear icon to workspace switcher

Owner-only settings gear navigates to /workspaces/[name]/settings.
Hidden for non-owner roles via useWorkspacePermissions.
EOF
```

---

### Task 10: Add services field to WorkspaceSpec type

The ServicesTab needs `spec.services` which may be missing from the TS types.

**Files:**
- Modify: `dashboard/src/types/workspace.ts`

- [ ] **Step 1: Check if services exists on WorkspaceSpec**

Search `dashboard/src/types/workspace.ts` for `services` in the `WorkspaceSpec` interface. If missing, add it.

- [ ] **Step 2: Add services field if missing**

Inside the `WorkspaceSpec` interface, add:

```typescript
  /** Per-workspace service groups for session-api and memory-api */
  services?: Array<{
    name: string;
    mode: "managed" | "external";
    session?: {
      database?: {
        secretRef?: { name: string };
      };
    };
    memory?: {
      database?: {
        secretRef?: { name: string };
      };
    };
  }>;
```

- [ ] **Step 3: Verify typecheck passes**

Run: `cd dashboard && npm run typecheck`
Expected: PASS

- [ ] **Step 4: Commit**

```
git add dashboard/src/types/workspace.ts
```
```
cat <<'EOF' | git commit -F -
fix(dashboard): add services field to WorkspaceSpec type

Needed by the workspace settings Services tab to display service group
configuration alongside status.
EOF
```

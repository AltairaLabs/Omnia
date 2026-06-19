import { describe, it, expect, beforeEach, vi, Mock } from "vitest";
import { NextRequest } from "next/server";
import { GET, PATCH } from "./route";
import type { Workspace, WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

vi.mock("@/lib/auth/index", () => ({ getUser: vi.fn() }));
vi.mock("@/lib/auth/workspace-authz", () => ({ checkWorkspaceAccess: vi.fn() }));
vi.mock("@/lib/k8s/workspace-client", () => ({
  getWorkspace: vi.fn(),
  patchWorkspace: vi.fn(),
}));

import { getUser } from "@/lib/auth/index";
import { checkWorkspaceAccess } from "@/lib/auth/workspace-authz";
import { getWorkspace, patchWorkspace } from "@/lib/k8s/workspace-client";

const ADMIN: User = { id: "a", username: "admin", groups: [], role: "admin", provider: "builtin" };

// Platform admin with no explicit grant: manage-only.
const MANAGE_ONLY: WorkspaceAccess = {
  granted: true,
  role: null,
  permissions: { read: false, write: false, delete: false, manageMembers: true },
};
const VIEWER: WorkspaceAccess = {
  granted: true,
  role: "viewer",
  permissions: { read: true, write: false, delete: false, manageMembers: false },
};
const DENIED: WorkspaceAccess = {
  granted: false,
  role: null,
  permissions: { read: false, write: false, delete: false, manageMembers: false },
};

const workspace: Workspace = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "Workspace",
  metadata: { name: "w", creationTimestamp: "2024-01-15T10:00:00Z" },
  spec: {
    displayName: "W",
    description: "",
    environment: "development",
    namespace: { name: "w-ns", create: true },
    roleBindings: [{ groups: ["owners"], role: "owner" }],
    directGrants: [],
  },
};

const ctx = (name = "w") => ({ params: Promise.resolve({ name }) });
const patchReq = (body: unknown) =>
  new NextRequest("http://localhost:3000/api/workspaces/w", {
    method: "PATCH",
    body: JSON.stringify(body),
    headers: { "content-type": "application/json" },
  });

describe("PATCH /api/workspaces/[name]", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (getUser as Mock).mockResolvedValue(ADMIN);
    (getWorkspace as Mock).mockResolvedValue(workspace);
    (patchWorkspace as Mock).mockResolvedValue({ ...workspace });
  });

  it("lets a platform admin (manage-only) edit access bindings", async () => {
    (checkWorkspaceAccess as Mock).mockResolvedValue(MANAGE_ONLY);
    const res = await PATCH(patchReq({ directGrants: [{ user: "admin", role: "owner" }] }), ctx());
    expect(res.status).toBe(200);
    expect(patchWorkspace).toHaveBeenCalled();
  });

  it("denies a viewer (no manageMembers)", async () => {
    (checkWorkspaceAccess as Mock).mockResolvedValue(VIEWER);
    const res = await PATCH(patchReq({ directGrants: [{ user: "x", role: "viewer" }] }), ctx());
    expect(res.status).toBe(403);
    expect(patchWorkspace).not.toHaveBeenCalled();
  });

  it("400s when no updatable fields are provided", async () => {
    (checkWorkspaceAccess as Mock).mockResolvedValue(MANAGE_ONLY);
    const res = await PATCH(patchReq({ displayName: "nope" }), ctx());
    expect(res.status).toBe(400);
  });

  it("409s when the change would remove all owner access", async () => {
    (checkWorkspaceAccess as Mock).mockResolvedValue(MANAGE_ONLY);
    // Replace bindings with a viewer-only set and clear grants → no owner path.
    const res = await PATCH(
      patchReq({ roleBindings: [{ groups: ["v"], role: "viewer" }], directGrants: [] }),
      ctx()
    );
    expect(res.status).toBe(409);
  });

  it("500s when the patch write fails", async () => {
    (checkWorkspaceAccess as Mock).mockResolvedValue(MANAGE_ONLY);
    (patchWorkspace as Mock).mockResolvedValue(null);
    const res = await PATCH(patchReq({ directGrants: [{ user: "admin", role: "owner" }] }), ctx());
    expect(res.status).toBe(500);
  });

  it("500s when the patch throws", async () => {
    (checkWorkspaceAccess as Mock).mockResolvedValue(MANAGE_ONLY);
    (patchWorkspace as Mock).mockRejectedValue(new Error("boom"));
    const res = await PATCH(patchReq({ directGrants: [{ user: "admin", role: "owner" }] }), ctx());
    expect(res.status).toBe(500);
  });
});

describe("GET /api/workspaces/[name]", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    (getUser as Mock).mockResolvedValue(ADMIN);
    (getWorkspace as Mock).mockResolvedValue(workspace);
  });

  it("lets a platform admin (manage-only) load the workspace with membership", async () => {
    (checkWorkspaceAccess as Mock).mockResolvedValue(MANAGE_ONLY);
    const res = await GET(new NextRequest("http://localhost:3000/api/workspaces/w"), ctx());
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.membership).toBeDefined();
  });

  it("returns the full CRD on ?view=full for a manage-capable caller", async () => {
    (checkWorkspaceAccess as Mock).mockResolvedValue(MANAGE_ONLY);
    const res = await GET(new NextRequest("http://localhost:3000/api/workspaces/w?view=full"), ctx());
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.kind).toBe("Workspace");
  });

  it("gives a viewer basic details without membership", async () => {
    (checkWorkspaceAccess as Mock).mockResolvedValue(VIEWER);
    const res = await GET(new NextRequest("http://localhost:3000/api/workspaces/w"), ctx());
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.membership).toBeUndefined();
  });

  it("403s a caller with no access", async () => {
    (checkWorkspaceAccess as Mock).mockResolvedValue(DENIED);
    const res = await GET(new NextRequest("http://localhost:3000/api/workspaces/w"), ctx());
    expect(res.status).toBe(403);
  });

  it("404s when the workspace disappears after the access check", async () => {
    (checkWorkspaceAccess as Mock).mockResolvedValue(MANAGE_ONLY);
    (getWorkspace as Mock).mockResolvedValue(null);
    const res = await GET(new NextRequest("http://localhost:3000/api/workspaces/w"), ctx());
    expect(res.status).toBe(404);
  });

  it("500s when the read throws", async () => {
    (checkWorkspaceAccess as Mock).mockResolvedValue(MANAGE_ONLY);
    (getWorkspace as Mock).mockRejectedValue(new Error("boom"));
    const res = await GET(new NextRequest("http://localhost:3000/api/workspaces/w"), ctx());
    expect(res.status).toBe(500);
  });
});

/** Tests for the agent external-exposure PATCH route (#1611). */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({ getUser: vi.fn() }));
vi.mock("@/lib/auth/workspace-authz", () => ({ checkWorkspaceAccess: vi.fn() }));
vi.mock("@/lib/k8s/workspace-route-helpers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/workspace-route-helpers")>();
  return { ...actual, getWorkspaceResource: vi.fn() };
});
vi.mock("@/lib/k8s/crd-operations", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/crd-operations")>();
  return { ...actual, patchCrd: vi.fn() };
});

const mockUser = {
  id: "u1",
  provider: "oauth" as const,
  username: "u",
  email: "u@example.com",
  groups: ["users"],
  role: "editor" as const,
};
const editorPerms = { read: true, write: true, delete: true, manageMembers: false };

function req(body: unknown): NextRequest {
  return new NextRequest("http://localhost:3000/api/workspaces/ws1/agents/a1/expose", {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}
function ctx() {
  return { params: Promise.resolve({ name: "ws1", agentName: "a1" }) };
}

async function setupEditor() {
  const { getUser } = await import("@/lib/auth");
  const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
  const { getWorkspaceResource } = await import("@/lib/k8s/workspace-route-helpers");
  vi.mocked(getUser).mockResolvedValue(mockUser);
  vi.mocked(checkWorkspaceAccess).mockResolvedValue({
    granted: true,
    role: "editor",
    permissions: editorPerms,
  });
  vi.mocked(getWorkspaceResource).mockResolvedValue({
    ok: true,
    resource: {} as never,
    workspace: {} as never,
    clientOptions: { workspace: "ws1", namespace: "ns1", role: "editor" },
  } as Awaited<ReturnType<typeof getWorkspaceResource>>);
}

describe("PATCH /workspaces/:name/agents/:agentName/expose (#1611)", () => {
  beforeEach(() => vi.clearAllMocks());

  it("patches spec.facade.expose with enabled + host", async () => {
    await setupEditor();
    const { patchCrd } = await import("@/lib/k8s/crd-operations");
    vi.mocked(patchCrd).mockResolvedValue({ metadata: { name: "a1" } } as never);
    const { PATCH } = await import("./route");

    const res = await PATCH(req({ enabled: true, host: "x.example.com" }), ctx());
    expect(res.status).toBe(200);
    expect(patchCrd).toHaveBeenCalledWith(expect.anything(), expect.anything(), "a1", {
      spec: { facade: { expose: { enabled: true, host: "x.example.com" } } },
    });
  });

  it("clears the host (null) when blank", async () => {
    await setupEditor();
    const { patchCrd } = await import("@/lib/k8s/crd-operations");
    vi.mocked(patchCrd).mockResolvedValue({} as never);
    const { PATCH } = await import("./route");

    await PATCH(req({ enabled: false, host: "  " }), ctx());
    expect(patchCrd).toHaveBeenCalledWith(expect.anything(), expect.anything(), "a1", {
      spec: { facade: { expose: { enabled: false, host: null } } },
    });
  });

  it("rejects a non-boolean enabled with 400", async () => {
    await setupEditor();
    const { PATCH } = await import("./route");
    const res = await PATCH(req({ enabled: "yes" }), ctx());
    expect(res.status).toBe(400);
  });
});

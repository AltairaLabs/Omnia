/**
 * Tests for the deploy-profile discovery endpoint. Issue #1519.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({ getUser: vi.fn() }));
vi.mock("@/lib/auth/workspace-authz", () => ({ checkWorkspaceAccess: vi.fn() }));
vi.mock("@/lib/k8s/workspace-route-helpers", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/workspace-route-helpers")>();
  return {
    ...actual,
    validateWorkspace: vi.fn(),
    createAuditContext: vi.fn().mockReturnValue({}),
    auditSuccess: vi.fn(),
    auditError: vi.fn(),
  };
});
vi.mock("@/lib/k8s/crd-operations", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/k8s/crd-operations")>();
  return { ...actual, listCrd: vi.fn() };
});

const mockUser = {
  id: "u1",
  provider: "oauth" as const,
  username: "u",
  email: "u@example.com",
  groups: ["users"],
  role: "viewer" as const,
};
const viewerPerms = { read: true, write: false, delete: false, manageMembers: false };
const noPerms = { read: false, write: false, delete: false, manageMembers: false };
const mockWorkspace = { metadata: { name: "test-ws" }, spec: { namespace: { name: "test-ns" } } };

function req(headers?: Record<string, string>): NextRequest {
  return new NextRequest("http://localhost:3000/api/workspaces/test-ws/deploy-profile", {
    method: "GET",
    headers,
  });
}
function ctx() {
  return { params: Promise.resolve({ name: "test-ws" }) };
}

async function setupAuthorized() {
  const { getUser } = await import("@/lib/auth");
  const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
  const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
  vi.mocked(getUser).mockResolvedValue(mockUser);
  vi.mocked(checkWorkspaceAccess).mockResolvedValue({
    granted: true,
    role: "viewer",
    permissions: viewerPerms,
  });
  vi.mocked(validateWorkspace).mockResolvedValue({
    ok: true,
    workspace: mockWorkspace,
    clientOptions: { workspace: "test-ws", namespace: "test-ns", role: "viewer" },
  } as Awaited<ReturnType<typeof validateWorkspace>>);
}

describe("GET /api/workspaces/[name]/deploy-profile", () => {
  beforeEach(() => vi.resetModules());
  afterEach(() => vi.resetAllMocks());

  it("returns 403 when user lacks workspace access", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: false,
      role: null,
      permissions: noPerms,
    });
    const { GET } = await import("./route");
    const res = await GET(req(), ctx());
    expect(res.status).toBe(403);
  });

  const ready = { status: { phase: "Ready" } };

  it("returns discovery shape mapped from Provider/SkillSource CRDs", async () => {
    await setupAuthorized();
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    vi.mocked(listCrd)
      .mockResolvedValueOnce([
        { metadata: { name: "default" }, spec: { type: "claude", role: "llm", model: "claude-sonnet-4" }, ...ready },
        { metadata: { name: "embedder" }, spec: { type: "openai", role: "embedding", model: "text-embed-3" }, ...ready },
        { metadata: { name: "legacy" }, spec: { type: "claude" }, ...ready },
      ] as never)
      .mockResolvedValueOnce([
        { metadata: { name: "docs-search" }, spec: { type: "git" }, ...ready },
      ] as never);
    const { GET } = await import("./route");
    const res = await GET(
      req({ "x-forwarded-host": "omnia.example.com", "x-forwarded-proto": "https" }),
      ctx()
    );
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.api_endpoint).toBe("https://omnia.example.com");
    expect(body.workspace).toBe("test-ws");
    expect(body.providers).toEqual([
      { name: "default", role: "llm", type: "claude", model: "claude-sonnet-4" },
      { name: "embedder", role: "embedding", type: "openai", model: "text-embed-3" },
      { name: "legacy", role: "llm", type: "claude" },
    ]);
    expect(body.skills).toEqual([{ name: "docs-search", type: "git" }]);
  });

  it("excludes Providers/SkillSources that are not Ready (#1519)", async () => {
    await setupAuthorized();
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    vi.mocked(listCrd)
      .mockResolvedValueOnce([
        { metadata: { name: "ready-llm" }, spec: { type: "claude", role: "llm" }, ...ready },
        { metadata: { name: "down-llm" }, spec: { type: "claude", role: "llm" }, status: { phase: "Unavailable" } },
        { metadata: { name: "no-status" }, spec: { type: "claude", role: "llm" } },
      ] as never)
      .mockResolvedValueOnce([
        { metadata: { name: "ready-skill" }, spec: { type: "git" }, ...ready },
        { metadata: { name: "erroring-skill" }, spec: { type: "git" }, status: { phase: "Error" } },
      ] as never);
    const { GET } = await import("./route");
    const res = await GET(req({ "x-forwarded-host": "omnia.example.com" }), ctx());
    const body = await res.json();
    expect(body.providers.map((p: { name: string }) => p.name)).toEqual(["ready-llm"]);
    expect(body.skills.map((s: { name: string }) => s.name)).toEqual(["ready-skill"]);
  });

  it("returns empty arrays for an empty workspace", async () => {
    await setupAuthorized();
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    vi.mocked(listCrd).mockResolvedValue([] as never);
    const { GET } = await import("./route");
    const res = await GET(req({ "x-forwarded-host": "omnia.example.com" }), ctx());
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.providers).toEqual([]);
    expect(body.skills).toEqual([]);
  });

  it("returns 500 when listing CRDs fails", async () => {
    await setupAuthorized();
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    vi.mocked(listCrd).mockRejectedValue(new Error("k8s down"));
    const { GET } = await import("./route");
    const res = await GET(req({ "x-forwarded-host": "omnia.example.com" }), ctx());
    expect(res.status).toBe(500);
  });

  it("returns the workspace-not-found response from validateWorkspace", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { validateWorkspace } = await import("@/lib/k8s/workspace-route-helpers");
    const { NextResponse } = await import("next/server");
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: true,
      role: "viewer",
      permissions: viewerPerms,
    });
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: false,
      response: NextResponse.json({ error: "Not Found" }, { status: 404 }),
    } as Awaited<ReturnType<typeof validateWorkspace>>);
    const { GET } = await import("./route");
    const res = await GET(req({ "x-forwarded-host": "omnia.example.com" }), ctx());
    expect(res.status).toBe(404);
  });

  it("yields an empty api_endpoint when no forwarded host and no env override", async () => {
    await setupAuthorized();
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    vi.mocked(listCrd).mockResolvedValue([] as never);
    const { GET } = await import("./route");
    const res = await GET(req(), ctx());
    const body = await res.json();
    expect(body.api_endpoint).toBe("");
  });

  it("falls back to env URL when no forwarded host header", async () => {
    vi.stubEnv("OMNIA_DASHBOARD_EXTERNAL_URL", "https://env.example.com");
    await setupAuthorized();
    const { listCrd } = await import("@/lib/k8s/crd-operations");
    vi.mocked(listCrd).mockResolvedValue([] as never);
    const { GET } = await import("./route");
    const res = await GET(req(), ctx());
    const body = await res.json();
    expect(body.api_endpoint).toBe("https://env.example.com");
    vi.unstubAllEnvs();
  });
});

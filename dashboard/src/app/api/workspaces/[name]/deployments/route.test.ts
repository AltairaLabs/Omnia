/**
 * Tests for the deployments proxy route. Issue #1866.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({ getUser: vi.fn() }));
vi.mock("@/lib/auth/workspace-authz", () => ({ checkWorkspaceAccess: vi.fn() }));
vi.mock("@/lib/data/deploy-api-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/data/deploy-api-service")>();
  return { ...actual, postDeployment: vi.fn() };
});

const mockUser = {
  id: "u1",
  provider: "oauth" as const,
  username: "u",
  email: "u@example.com",
  groups: ["users"],
  role: "editor" as const,
};
const editorPerms = { read: true, write: true, delete: false, manageMembers: false };
const noPerms = { read: false, write: false, delete: false, manageMembers: false };

function req(body?: unknown, rawBody?: string): NextRequest {
  return new NextRequest("http://localhost:3000/api/workspaces/test-ws/deployments", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: rawBody === undefined ? JSON.stringify(body ?? {}) : rawBody,
  });
}

function ctx() {
  return { params: Promise.resolve({ name: "test-ws" }) };
}

async function setupAuthorized() {
  const { getUser } = await import("@/lib/auth");
  const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
  vi.mocked(getUser).mockResolvedValue(mockUser);
  vi.mocked(checkWorkspaceAccess).mockResolvedValue({
    granted: true,
    role: "editor",
    permissions: editorPerms,
  });
}

describe("POST /api/workspaces/[name]/deployments", () => {
  beforeEach(() => vi.resetModules());
  afterEach(() => vi.resetAllMocks());

  it("forwards an authorized editor's intent and returns the service status + result", async () => {
    await setupAuthorized();
    const { postDeployment } = await import("@/lib/data/deploy-api-service");
    const result = { succeeded: true, results: [{ kind: "AgentRuntime", name: "a", action: "created" }] };
    vi.mocked(postDeployment).mockResolvedValue({ status: 200, result });

    const intent = { apiVersion: "deploy.omnia.altairalabs.ai/v1", kind: "DeployIntent" };
    const { POST } = await import("./route");
    const res = await POST(req(intent), ctx());

    expect(postDeployment).toHaveBeenCalledWith("test-ws", mockUser, intent);
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual(result);
  });

  it("passes through a 207 multi-status response from the service", async () => {
    await setupAuthorized();
    const { postDeployment } = await import("@/lib/data/deploy-api-service");
    const result = {
      succeeded: false,
      results: [{ kind: "AgentRuntime", name: "a", action: "error", error: "boom" }],
    };
    vi.mocked(postDeployment).mockResolvedValue({ status: 207, result });

    const { POST } = await import("./route");
    const res = await POST(req({ kind: "DeployIntent" }), ctx());

    expect(res.status).toBe(207);
    expect(await res.json()).toEqual(result);
  });

  it("maps a DeployApiError(403) from the service to a 403 error response", async () => {
    await setupAuthorized();
    const { postDeployment, DeployApiError } = await import("@/lib/data/deploy-api-service");
    vi.mocked(postDeployment).mockRejectedValue(new DeployApiError("forbidden", 403));

    const { POST } = await import("./route");
    const res = await POST(req({ kind: "DeployIntent" }), ctx());

    expect(res.status).toBe(403);
    expect(await res.json()).toEqual({ error: "forbidden" });
  });

  it("maps a DeployApiError(400) from the service to a 400 error response", async () => {
    await setupAuthorized();
    const { postDeployment, DeployApiError } = await import("@/lib/data/deploy-api-service");
    vi.mocked(postDeployment).mockRejectedValue(new DeployApiError("bad intent", 400));

    const { POST } = await import("./route");
    const res = await POST(req({ kind: "DeployIntent" }), ctx());

    expect(res.status).toBe(400);
    expect(await res.json()).toEqual({ error: "bad intent" });
  });

  it("re-throws a non-DeployApiError from the service instead of swallowing it", async () => {
    await setupAuthorized();
    const { postDeployment } = await import("@/lib/data/deploy-api-service");
    vi.mocked(postDeployment).mockRejectedValue(new Error("network exploded"));

    const { POST } = await import("./route");
    await expect(POST(req({ kind: "DeployIntent" }), ctx())).rejects.toThrow("network exploded");
  });

  it("returns 403 for a viewer and never calls postDeployment", async () => {
    const { getUser } = await import("@/lib/auth");
    const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
    const { postDeployment } = await import("@/lib/data/deploy-api-service");
    vi.mocked(getUser).mockResolvedValue(mockUser);
    vi.mocked(checkWorkspaceAccess).mockResolvedValue({
      granted: false,
      role: "viewer",
      permissions: noPerms,
    });

    const { POST } = await import("./route");
    const res = await POST(req({ kind: "DeployIntent" }), ctx());

    expect(res.status).toBe(403);
    expect(postDeployment).not.toHaveBeenCalled();
  });

  it("returns 400 for an invalid JSON body and never calls postDeployment", async () => {
    await setupAuthorized();
    const { postDeployment } = await import("@/lib/data/deploy-api-service");

    const { POST } = await import("./route");
    const res = await POST(req(undefined, "not json"), ctx());

    expect(res.status).toBe(400);
    expect(await res.json()).toEqual({ error: "invalid JSON body" });
    expect(postDeployment).not.toHaveBeenCalled();
  });
});

/**
 * Tests for the function-mode AgentRuntime invoke proxy route.
 *
 * The critical contract: the minted mgmt-plane JWT must carry the workspace
 * NAME (the route's `[name]` param), not the K8s namespace. Minting with the
 * namespace 401s at the facade whenever name != namespace (#1552, same class
 * as the WS proxy bug).
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("@/lib/auth/workspace-guard", () => ({
  withWorkspaceAccess: (_role: string, handler: unknown) => handler,
}));
vi.mock("@/lib/k8s/workspace-route-helpers", () => ({
  getWorkspaceResource: vi.fn(),
  CRD_AGENTS: "agentruntimes",
}));
vi.mock("@/lib/functions/invoke-token", () => ({
  mgmtPlaneAuthHeaders: vi.fn(() => ({})),
}));
vi.mock("@/types/agent-runtime", () => ({
  isFunctionMode: vi.fn(() => true),
}));

import { POST } from "./route";
import { getWorkspaceResource } from "@/lib/k8s/workspace-route-helpers";
import { mgmtPlaneAuthHeaders } from "@/lib/functions/invoke-token";
import { isFunctionMode } from "@/types/agent-runtime";

type Handler = (r: Request, c: unknown, a: unknown, u: unknown) => Promise<Response>;

// [name] is the workspace NAME ("ws"); the backing K8s namespace ("omnia-ws")
// lives on the Workspace's spec.namespace.name — deliberately different to
// catch the confusion.
const ctx = { params: Promise.resolve({ name: "ws", fnName: "fn" }) };
const access = { granted: true, role: "editor" };
const user = { email: "admin@example.com" };

function postReq(): Request {
  return new Request("https://localhost/api/workspaces/ws/functions/fn/invoke", {
    method: "POST",
    body: JSON.stringify({ input: "hi" }),
  });
}

function invoke(): Promise<Response> {
  return (POST as unknown as Handler)(postReq(), ctx, access, user);
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(isFunctionMode).mockReturnValue(true);
  vi.mocked(getWorkspaceResource).mockResolvedValue({
    ok: true,
    resource: { spec: { facade: { port: 8080 } } },
    workspace: { spec: { namespace: { name: "omnia-ws" } } },
  } as Awaited<ReturnType<typeof getWorkspaceResource>>);
});

describe("POST function invoke proxy", () => {
  it("mints the mgmt-plane token with the workspace name, not the namespace", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(JSON.stringify({ output: "ok" }), { status: 200 })),
    );

    await invoke();

    expect(mgmtPlaneAuthHeaders).toHaveBeenCalledWith("fn", "ws", "admin@example.com");
    vi.unstubAllGlobals();
  });

  it("returns 400 when the AgentRuntime is not function-mode", async () => {
    vi.mocked(isFunctionMode).mockReturnValue(false);
    const res = await invoke();
    expect(res.status).toBe(400);
  });

  it("returns 502 when the facade is unreachable", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new Error("ECONNREFUSED");
      }),
    );
    const res = await invoke();
    expect(res.status).toBe(502);
    vi.unstubAllGlobals();
  });

  it("passes the facade response status and body straight through", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("bad input", { status: 422 })),
    );
    const res = await invoke();
    expect(res.status).toBe(422);
    expect(await res.text()).toBe("bad input");
    vi.unstubAllGlobals();
  });

  it("surfaces the resource-resolution error response when lookup fails", async () => {
    const failResponse = new Response("nope", { status: 404 });
    vi.mocked(getWorkspaceResource).mockResolvedValue({
      ok: false,
      response: failResponse,
    } as Awaited<ReturnType<typeof getWorkspaceResource>>);
    const res = await invoke();
    expect(res.status).toBe(404);
  });
});

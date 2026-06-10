/**
 * Tests for the ToolRegistry tool-test proxy route.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("@/lib/auth/workspace-guard", () => ({
  withWorkspaceAccess: (_role: string, handler: unknown) => handler,
}));
vi.mock("@/lib/k8s/workspace-route-helpers", () => ({
  validateWorkspace: vi.fn(),
}));
vi.mock("node:fs/promises", () => {
  const readFile = vi.fn();
  return { readFile, default: { readFile } };
});

import { POST } from "./route";
import { validateWorkspace } from "@/lib/k8s/workspace-route-helpers";
import { readFile } from "node:fs/promises";

type Handler = (r: Request, c: unknown, a: unknown, u: unknown) => Promise<Response>;
const ctx = { params: Promise.resolve({ name: "ws", registryName: "reg" }) };
const access = { granted: true, role: "editor" };

function postReq(): Request {
  return new Request("https://localhost/api/workspaces/ws/toolregistries/reg/test", {
    method: "POST",
    body: JSON.stringify({ handlerName: "h", arguments: {} }),
  });
}

function invoke(): Promise<Response> {
  return (POST as unknown as Handler)(postReq(), ctx, access, {});
}

beforeEach(() => {
  vi.clearAllMocks();
  delete process.env.OPERATOR_TOOL_TEST_TOKEN;
  vi.mocked(validateWorkspace).mockResolvedValue({
    ok: true,
    workspace: { spec: { namespace: { name: "omnia-ws" } } },
  } as Awaited<ReturnType<typeof validateWorkspace>>);
});

describe("POST tool-test proxy", () => {
  it("forwards the pod ServiceAccount token as a bearer header", async () => {
    vi.mocked(readFile).mockResolvedValue("sa-token-123\n" as never);
    const seen: Array<{ headers: Record<string, string> }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url: string, opts: { headers: Record<string, string> }) => {
        seen.push(opts);
        return new Response(JSON.stringify({ success: true }), { status: 200 });
      })
    );

    const res = await invoke();
    expect(res.status).toBe(200);
    expect(seen[0].headers.Authorization).toBe("Bearer sa-token-123");
    vi.unstubAllGlobals();
  });

  it("omits the auth header when no token is available (local dev)", async () => {
    vi.mocked(readFile).mockRejectedValue(new Error("ENOENT"));
    const seen: Array<{ headers: Record<string, string> }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url: string, opts: { headers: Record<string, string> }) => {
        seen.push(opts);
        return new Response(JSON.stringify({ success: true }), { status: 200 });
      })
    );

    await invoke();
    expect(seen[0].headers.Authorization).toBeUndefined();
    vi.unstubAllGlobals();
  });

  it("prefers an explicit OPERATOR_TOOL_TEST_TOKEN env over the SA file", async () => {
    process.env.OPERATOR_TOOL_TEST_TOKEN = "explicit-token";
    const seen: Array<{ headers: Record<string, string> }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_url: string, opts: { headers: Record<string, string> }) => {
        seen.push(opts);
        return new Response(JSON.stringify({ success: true }), { status: 200 });
      })
    );

    await invoke();
    expect(seen[0].headers.Authorization).toBe("Bearer explicit-token");
    expect(readFile).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });
});

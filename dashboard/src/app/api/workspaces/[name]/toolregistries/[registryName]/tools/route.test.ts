/**
 * Tests for the ToolRegistry live tool-list proxy route.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("@/lib/auth/workspace-guard", () => ({
  withWorkspaceAccess: (_role: string, handler: unknown) => handler,
}));
vi.mock("@/lib/k8s/workspace-route-helpers", () => ({ validateWorkspace: vi.fn() }));
vi.mock("@/lib/tooltest/operator-client", () => ({
  OPERATOR_TOOL_TEST_URL: "https://operator:8083",
  operatorAuthToken: vi.fn().mockResolvedValue("sa-token"),
}));

import { GET } from "./route";
import { validateWorkspace } from "@/lib/k8s/workspace-route-helpers";

type Handler = (r: Request, c: unknown, a: unknown, u: unknown) => Promise<Response>;
const access = { role: "editor" };
const user = {};
const ctx = () => ({ params: Promise.resolve({ name: "ws", registryName: "reg" }) });

beforeEach(() => {
  vi.clearAllMocks();
  (validateWorkspace as ReturnType<typeof vi.fn>).mockResolvedValue({
    ok: true, workspace: { spec: { namespace: { name: "ns" } } },
  });
});

describe("GET tools proxy", () => {
  it("forwards the tool list and the SA bearer token", async () => {
    const fetchMock = vi.spyOn(global, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ tools: [{ name: "getPet", description: "d" }] }), { status: 200 })
    );
    const req = new Request("https://x/api?handler=petstore");
    const res = await (GET as unknown as Handler)(req, ctx(), access, user);
    expect((await res.json()).tools[0].name).toBe("getPet");
    const call = fetchMock.mock.calls[0];
    expect(call[0]).toContain("/api/v1/namespaces/ns/toolregistries/reg/tools?handler=petstore");
    expect((call[1] as RequestInit).headers).toMatchObject({ Authorization: "Bearer sa-token" });
  });

  it("returns an error payload when the operator is unreachable", async () => {
    vi.spyOn(global, "fetch").mockRejectedValue(new Error("ECONNREFUSED"));
    const req = new Request("https://x/api?handler=petstore");
    const res = await (GET as unknown as Handler)(req, ctx(), access, user);
    expect((await res.json()).error).toMatch(/unreachable/i);
  });
});

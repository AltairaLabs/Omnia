/**
 * Tests for the workspace costs route (reads exact data from session-api).
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { NextResponse } from "next/server";

// Pass-through the access guard so we can drive the handler directly.
vi.mock("@/lib/auth/workspace-guard", () => ({
  withWorkspaceAccess: (_role: string, handler: unknown) => handler,
}));

vi.mock("@/lib/k8s/service-url-resolver", () => ({
  resolveServiceURLs: vi.fn(),
}));

vi.mock("@/lib/k8s/workspace-route-helpers", () => ({
  validateWorkspace: vi.fn(),
  serverErrorResponse: (e: unknown) =>
    NextResponse.json({ error: "Internal Server Error", message: String(e) }, { status: 500 }),
  createAuditContext: vi.fn(() => ({})),
  auditSuccess: vi.fn(),
  auditError: vi.fn(),
}));

import { GET } from "./route";
import { resolveServiceURLs } from "@/lib/k8s/service-url-resolver";
import { validateWorkspace } from "@/lib/k8s/workspace-route-helpers";

type Handler = (r: unknown, c: unknown, a: unknown, u: unknown) => Promise<NextResponse>;

const ctx = { params: Promise.resolve({ name: "default" }) };
const access = { granted: true, role: "viewer" };
const user = { id: "u1" };

function invoke(): Promise<NextResponse> {
  return (GET as unknown as Handler)(
    new Request("https://localhost/api/workspaces/default/costs"),
    ctx,
    access,
    user,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(resolveServiceURLs).mockResolvedValue({
    sessionURL: "https://session-default:8080",
    memoryURL: "https://memory-default:8080",
    namespace: "omnia-default",
  });
  vi.mocked(validateWorkspace).mockResolvedValue({
    ok: true,
    workspace: {
      spec: { namespace: { name: "omnia-default" }, costControls: { dailyBudget: "10" } },
    },
    clientOptions: {},
  } as never);
});

describe("GET /api/workspaces/[name]/costs", () => {
  it("assembles CostData from session-api and pins the resolved namespace", async () => {
    const calls: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string) => {
        calls.push(url);
        const rows = url.includes("time%3Ahour")
          ? [{ key: "2026-06-09T13:00:00Z|openai", value: 0.03, count: 2 }]
          : [{ key: "openai|gpt-4|chatbot", value: url.includes("sum_cost_usd") ? 0.03 : 2, count: 2 }];
        return new Response(JSON.stringify({ rows }), { status: 200 });
      }),
    );

    const res = await invoke();
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.summary.totalCost).toBeCloseTo(0.03, 9);
    expect(body.budget?.dailyUsedPercent).toBeCloseTo((0.03 / 10) * 100, 6);
    expect(calls.every((u) => u.includes("namespace=omnia-default"))).toBe(true);
    vi.unstubAllGlobals();
  });

  it("returns available:false when session-api is not configured", async () => {
    vi.mocked(resolveServiceURLs).mockResolvedValue(null);
    const res = await invoke();
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body.available).toBe(false);
    expect(body.reason).toBeTruthy();
  });

  it("propagates the validateWorkspace failure response", async () => {
    vi.mocked(validateWorkspace).mockResolvedValue({
      ok: false,
      response: NextResponse.json({ error: "not found" }, { status: 404 }),
    } as never);
    const res = await invoke();
    expect(res.status).toBe(404);
  });
});

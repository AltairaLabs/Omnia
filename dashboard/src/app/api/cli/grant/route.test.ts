import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({ getUser: vi.fn() }));
vi.mock("@/lib/auth/session-store", () => ({ getSessionStore: vi.fn() }));
vi.mock("@/lib/auth/workspace-authz", () => ({ checkWorkspaceAccess: vi.fn() }));

const store = { getCliFlow: vi.fn(), consumeCliFlow: vi.fn(), putCliCode: vi.fn() };
const editorUser = { id: "u1", provider: "oauth", email: "u@e.com", username: "u", groups: ["g"], role: "editor" };
const flowRec = { callback: "http://127.0.0.1:5000/cb", cliState: "abcd1234", createdAt: 1 };

function formReq(body: Record<string, string>, headers: Record<string, string> = {}): NextRequest {
  const params = new URLSearchParams(body);
  return new NextRequest("https://omnia.example.com/api/cli/grant", {
    method: "POST",
    headers: { "content-type": "application/x-www-form-urlencoded", "x-forwarded-host": "omnia.example.com", ...headers },
    body: params.toString(),
  });
}

async function arrange(opts: { user?: unknown; flow?: unknown; granted?: boolean } = {}) {
  const { getUser } = await import("@/lib/auth");
  const { getSessionStore } = await import("@/lib/auth/session-store");
  const { checkWorkspaceAccess } = await import("@/lib/auth/workspace-authz");
  vi.mocked(getUser).mockResolvedValue((opts.user ?? editorUser) as never);
  vi.mocked(getSessionStore).mockReturnValue(store as never);
  store.getCliFlow.mockResolvedValue(opts.flow === undefined ? flowRec : opts.flow);
  store.consumeCliFlow.mockResolvedValue(flowRec);
  vi.mocked(checkWorkspaceAccess).mockResolvedValue({
    granted: opts.granted ?? true,
    role: (opts.granted ?? true) ? "editor" : null,
    permissions: {} as never,
  });
}

describe("POST /api/cli/grant", () => {
  beforeEach(() => { vi.resetModules(); Object.values(store).forEach((m) => m.mockReset()); });
  afterEach(() => vi.resetAllMocks());

  it("403 on cross-origin", async () => {
    await arrange();
    const { POST } = await import("./route");
    const res = await POST(formReq({ flow: "f1", workspace: "team-acme" }, { origin: "https://evil.com" }));
    expect(res.status).toBe(403);
  });

  it("401 for an anonymous user", async () => {
    await arrange({ user: { provider: "anonymous" } });
    const { POST } = await import("./route");
    expect((await POST(formReq({ flow: "f1", workspace: "team-acme" }))).status).toBe(401);
  });

  it("400 when the flow is missing/expired", async () => {
    await arrange({ flow: null });
    const { POST } = await import("./route");
    expect((await POST(formReq({ flow: "f1", workspace: "team-acme" }))).status).toBe(400);
  });

  it("403 when the user lacks editor on the workspace", async () => {
    await arrange({ granted: false });
    const { POST } = await import("./route");
    expect((await POST(formReq({ flow: "f1", workspace: "team-acme" }))).status).toBe(403);
  });

  it("mints a code, consumes the flow, and 303-redirects to the loopback callback", async () => {
    await arrange();
    const { POST } = await import("./route");
    const res = await POST(formReq({ flow: "f1", workspace: "team-acme" }));
    expect(res.status).toBe(303);
    const loc = new URL(res.headers.get("location")!);
    expect(loc.origin).toBe("http://127.0.0.1:5000");
    expect(loc.searchParams.get("state")).toBe("abcd1234");
    expect(loc.searchParams.get("code")).toBeTruthy();
    expect(store.putCliCode).toHaveBeenCalledWith(
      loc.searchParams.get("code"),
      expect.objectContaining({ userId: "u1", workspace: "team-acme", userRole: "editor", workspaceRole: "editor" }),
      60
    );
    expect(store.consumeCliFlow).toHaveBeenCalledWith("f1");
  });
});

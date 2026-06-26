import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

vi.mock("@/lib/auth", () => ({ getUser: vi.fn() }));
vi.mock("@/lib/auth/config", () => ({ getAuthConfig: vi.fn() }));
vi.mock("@/lib/auth/session-store", () => ({ getSessionStore: vi.fn() }));

const putCliFlow = vi.fn();
const oauthConfig = {
  mode: "oauth",
  baseUrl: "https://omnia.example.com",
  session: { pkceTtl: 300 },
};

function req(qs: string): NextRequest {
  return new NextRequest(`https://omnia.example.com/api/cli/authorize${qs}`);
}

async function arrange(opts: { mode?: string; authed?: boolean } = {}) {
  const { getAuthConfig } = await import("@/lib/auth/config");
  const { getSessionStore } = await import("@/lib/auth/session-store");
  const { getUser } = await import("@/lib/auth");
  vi.mocked(getAuthConfig).mockReturnValue({ ...oauthConfig, mode: opts.mode ?? "oauth" } as never);
  vi.mocked(getSessionStore).mockReturnValue({ putCliFlow } as never);
  if (opts.authed) vi.mocked(getUser).mockResolvedValue({ provider: "oauth" } as never);
  else vi.mocked(getUser).mockResolvedValue({ provider: "anonymous" } as never);
}

const OK = "?callback=http%3A%2F%2F127.0.0.1%3A5000%2Fcb&state=abcd1234";

describe("GET /api/cli/authorize", () => {
  beforeEach(() => { vi.resetModules(); putCliFlow.mockReset(); });
  afterEach(() => vi.resetAllMocks());

  it("400 when not in oauth mode", async () => {
    await arrange({ mode: "anonymous" });
    const { GET } = await import("./route");
    expect((await GET(req(OK))).status).toBe(400);
  });

  it("400 on a non-loopback callback", async () => {
    await arrange();
    const { GET } = await import("./route");
    const res = await GET(req("?callback=https%3A%2F%2Fevil.com%2Fcb&state=abcd1234"));
    expect(res.status).toBe(400);
  });

  it("400 on a bad state", async () => {
    await arrange();
    const { GET } = await import("./route");
    expect((await GET(req("?callback=http%3A%2F%2F127.0.0.1%3A5000%2Fcb&state=x"))).status).toBe(400);
  });

  it("stashes the flow and redirects an authed user to the picker", async () => {
    await arrange({ authed: true });
    const { GET } = await import("./route");
    const res = await GET(req(OK));
    expect(res.status).toBe(307);
    const loc = res.headers.get("location")!;
    expect(loc).toContain("/cli/select?flow=");
    expect(putCliFlow).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ callback: "http://127.0.0.1:5000/cb", cliState: "abcd1234" }),
      300
    );
  });

  it("redirects an unauthed user to login with a returnTo to the picker", async () => {
    await arrange({ authed: false });
    const { GET } = await import("./route");
    const loc = (await GET(req(OK))).headers.get("location")!;
    expect(loc).toContain("/api/auth/login?returnTo=");
    expect(decodeURIComponent(loc)).toContain("/cli/select?flow=");
  });
});

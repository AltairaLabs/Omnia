import { describe, it, expect, vi, beforeEach } from "vitest";
import { NextRequest } from "next/server";
import { MemorySessionStore } from "@/lib/auth/session-store/memory-store";

// Instantiate store at module level so the mock factory can close over it.
const store = new MemorySessionStore();

vi.mock("@/lib/auth/session-store", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth/session-store")>("@/lib/auth/session-store");
  return { ...actual, getSessionStore: () => store };
});

vi.mock("@/lib/auth/session", () => ({
  saveUserToSession: vi.fn(async () => undefined),
}));

vi.mock("@/lib/auth/oauth", () => ({
  exchangeCodeForTokens: vi.fn(async () => ({
    refresh_token: "rt", id_token: "it", expires_at: 999,
  })),
  extractClaims: () => ({ sub: "u1", email: "u1@example", name: "U1", groups: [] }),
  mapClaimsToUser: () => ({
    id: "u1", username: "u1", email: "u1@example", displayName: "U1",
    groups: [], role: "viewer", provider: "oauth",
  }),
  getUserInfo: vi.fn(),
  validateClaims: () => true,
}));

beforeEach(async () => {
  process.env.OMNIA_AUTH_MODE = "oauth";
  process.env.OMNIA_SESSION_SECRET = "a".repeat(32);
  process.env.OMNIA_BASE_URL = "https://omnia.example";
  await store.putPkce(
    "state-123",
    { codeVerifier: "v", codeChallenge: "c", state: "state-123", returnTo: "/dash", createdAt: Date.now() },
    300,
  );
});

function reqWithStateCookie(cookieState: string | null, urlState: string, code = "code-1"): NextRequest {
  const url = `https://omnia.example/api/auth/callback?code=${code}&state=${urlState}`;
  const req = new NextRequest(url);
  if (cookieState !== null) req.cookies.set("omnia_oauth_state", cookieState);
  return req;
}

describe("GET /api/auth/callback", () => {
  it("redirects to /login?error=invalid_state when the state cookie is missing", async () => {
    const { GET } = await import("./route");
    const res = await GET(reqWithStateCookie(null, "state-123"));
    expect(res.status).toBe(307);
    expect(res.headers.get("location")).toContain("error=invalid_state");
  });

  it("redirects to /login?error=invalid_state when cookie state != url state", async () => {
    const { GET } = await import("./route");
    const res = await GET(reqWithStateCookie("state-OTHER", "state-123"));
    expect(res.headers.get("location")).toContain("error=invalid_state");
  });

  it("consumes pkce, mints a sid, and redirects to returnTo", async () => {
    const { GET } = await import("./route");
    const res = await GET(reqWithStateCookie("state-123", "state-123"));
    expect(res.status).toBe(307);
    expect(res.headers.get("location")).toBe("https://omnia.example/dash");

    expect(await store.consumePkce("state-123")).toBeNull();

    const setCookie = res.headers.get("set-cookie") ?? "";
    expect(setCookie).toMatch(/omnia_oauth_state=;.*Max-Age=0/i);
  });

  it("rejects a replay where pkce has already been consumed", async () => {
    await store.consumePkce("state-123");
    const { GET } = await import("./route");
    const res = await GET(reqWithStateCookie("state-123", "state-123"));
    expect(res.headers.get("location")).toContain("error=invalid_state");
  });

  it("redirects with IdP error when error param is set", async () => {
    const req = new NextRequest("https://omnia.example/api/auth/callback?error=access_denied");
    const { GET } = await import("./route");
    const res = await GET(req);
    expect(res.headers.get("location")).toContain("error=access_denied");
  });

  it("redirects with no_code when code is missing", async () => {
    const req = new NextRequest("https://omnia.example/api/auth/callback?state=state-123");
    req.cookies.set("omnia_oauth_state", "state-123");
    const { GET } = await import("./route");
    const res = await GET(req);
    expect(res.headers.get("location")).toContain("error=no_code");
  });
});

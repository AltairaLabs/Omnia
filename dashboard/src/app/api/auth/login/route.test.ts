import { describe, it, expect, vi, beforeEach } from "vitest";
import { NextRequest } from "next/server";

// Create store via hoisted factory to avoid import-before-init issues.
// MemorySessionStore is instantiated inline without importing it at module scope.
const { store } = vi.hoisted(() => {
  // Inline minimal store to avoid the import-before-init constraint.
  type PkceRecord = {
    codeVerifier: string;
    codeChallenge: string;
    state: string;
    returnTo: string;
    createdAt: number;
  };
  interface Entry { value: PkceRecord; expiresAt: number }
  const pkce = new Map<string, Entry>();
  const store = {
    putPkce: async (state: string, record: PkceRecord, ttlSeconds: number) => {
      pkce.set(state, { value: record, expiresAt: Date.now() + ttlSeconds * 1000 });
    },
    consumePkce: async (state: string): Promise<PkceRecord | null> => {
      const entry = pkce.get(state);
      if (!entry) return null;
      pkce.delete(state);
      if (entry.expiresAt <= Date.now()) return null;
      return entry.value;
    },
    getSession: async () => null,
    putSession: async () => {},
    deleteSession: async () => {},
    _clear: () => pkce.clear(),
  };
  return { store };
});

vi.mock("@/lib/auth/session-store", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth/session-store")>(
    "@/lib/auth/session-store",
  );
  return { ...actual, getSessionStore: () => store };
});

vi.mock("@/lib/auth/oauth", () => ({
  generatePKCE: vi.fn(async (returnTo?: string) => ({
    codeVerifier: "v",
    codeChallenge: "c",
    state: "state-123",
    returnTo: returnTo ?? "/",
  })),
  buildAuthorizationUrl: vi.fn(async (pkce: { state: string }) => `https://idp.example/auth?state=${pkce.state}`),
}));

beforeEach(() => {
  process.env.OMNIA_AUTH_MODE = "oauth";
  process.env.OMNIA_SESSION_SECRET = "a".repeat(32);
  process.env.OMNIA_BASE_URL = "https://omnia.example";
  store._clear();
});

describe("GET /api/auth/login", () => {
  it("writes pkce to the store keyed by state", async () => {
    const { GET } = await import("./route");
    const req = new NextRequest("https://omnia.example/api/auth/login?returnTo=/dash");
    const res = await GET(req);
    expect(res.status).toBe(307);
    expect(res.headers.get("location")).toContain("state=state-123");
    const consumed = await store.consumePkce("state-123");
    expect(consumed?.codeVerifier).toBe("v");
    expect(consumed?.returnTo).toBe("/dash");
  });

  it("sets ephemeral omnia_oauth_state cookie to the state value", async () => {
    const { GET } = await import("./route");
    const req = new NextRequest("https://omnia.example/api/auth/login");
    const res = await GET(req);
    const setCookie = res.headers.get("set-cookie") ?? "";
    expect(setCookie).toMatch(/omnia_oauth_state=state-123/);
    expect(setCookie).toMatch(/HttpOnly/i);
    expect(setCookie).toMatch(/SameSite=Lax/i);
  });

  it("returns 400 when not in oauth mode", async () => {
    process.env.OMNIA_AUTH_MODE = "builtin";
    const { GET } = await import("./route");
    const req = new NextRequest("https://omnia.example/api/auth/login");
    const res = await GET(req);
    expect(res.status).toBe(400);
  });
});

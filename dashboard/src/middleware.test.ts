import { describe, it, expect, vi, beforeEach } from "vitest";
import { NextRequest } from "next/server";
import { sealData } from "iron-session";

// Build the shared store inside vi.hoisted so it's available before any
// import is resolved (hoisted code runs before the module graph is built).
const { store } = vi.hoisted(() => {
  // Inline a minimal SessionStore-compatible object so we don't reference
  // MemorySessionStore (which isn't available at hoist time).
  type Entry = { value: unknown; expiresAt: number };
  const sessions = new Map<string, Entry>();
  const store = {
    async getSession(sid: string) {
      const e = sessions.get(sid);
      if (!e || e.expiresAt <= Date.now()) return null;
      return e.value as import("@/lib/auth/session-store").SessionRecord;
    },
    async putSession(sid: string, record: unknown, ttlSeconds: number) {
      sessions.set(sid, { value: record, expiresAt: Date.now() + ttlSeconds * 1000 });
    },
    async deleteSession(sid: string) { sessions.delete(sid); },
    async putPkce() { /* no-op */ },
    async consumePkce() { return null; },
  };
  return { store };
});

vi.mock("@/lib/auth/session-store", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth/session-store")>("@/lib/auth/session-store");
  return { ...actual, getSessionStore: () => store };
});

const SECRET = "a".repeat(32);

beforeEach(() => {
  process.env.OMNIA_AUTH_MODE = "oauth";
  process.env.OMNIA_SESSION_SECRET = SECRET;
  process.env.OMNIA_SESSION_COOKIE_NAME = "omnia_session";
  delete process.env.OMNIA_AUTH_API_KEYS_ENABLED;
});

async function reqWithSid(sid: string | null, path = "/dashboard"): Promise<NextRequest> {
  const url = `https://omnia.example${path}`;
  const req = new NextRequest(url);
  if (sid !== null) {
    const sealed = await sealData({ sid }, { password: SECRET });
    req.cookies.set("omnia_session", sealed);
  }
  return req;
}

const API_KEY = "omnia_sk_test123";
const WORKSPACES_API = "/api/workspaces/default/agents";

function reqWithHeaders(headers: Record<string, string>, path = WORKSPACES_API): NextRequest {
  return new NextRequest(`https://omnia.example${path}`, { headers });
}

describe("middleware", () => {
  it("redirects to /login when the cookie is missing", async () => {
    const { middleware } = await import("./middleware");
    const res = await middleware(await reqWithSid(null));
    expect(res.status).toBe(307);
    expect(res.headers.get("location")).toContain("/login");
  });

  it("lets the request through when sid resolves to an oauth user", async () => {
    await store.putSession("sid-1", {
      user: { id: "u", username: "u", groups: [], role: "viewer", provider: "oauth" },
      oauth: { provider: "azure" },
      createdAt: Date.now(),
    }, 60);
    const { middleware } = await import("./middleware");
    const res = await middleware(await reqWithSid("sid-1"));
    expect(res.status).toBe(200);
  });

  it("clears cookie and redirects when sid is missing from the store", async () => {
    const { middleware } = await import("./middleware");
    const res = await middleware(await reqWithSid("sid-missing"));
    expect(res.status).toBe(307);
    const setCookie = res.headers.get("set-cookie") ?? "";
    expect(setCookie).toMatch(/omnia_session=;.*Max-Age=0/i);
  });

  it("clears cookie when store returns a user whose provider != mode", async () => {
    await store.putSession("sid-wrong", {
      user: { id: "u", username: "u", groups: [], role: "viewer", provider: "builtin" },
      createdAt: Date.now(),
    }, 60);
    const { middleware } = await import("./middleware");
    const res = await middleware(await reqWithSid("sid-wrong"));
    expect(res.status).toBe(307);
  });

  it("passes anonymous mode requests through", async () => {
    process.env.OMNIA_AUTH_MODE = "anonymous";
    const { middleware } = await import("./middleware");
    const res = await middleware(await reqWithSid(null));
    expect(res.status).toBe(200);
  });

  it("lets public paths through without a cookie", async () => {
    const { middleware } = await import("./middleware");
    const res = await middleware(await reqWithSid(null, "/api/auth/login"));
    expect(res.status).toBe(200);
  });

  it("returns 401 JSON for unauthenticated API routes", async () => {
    const { middleware } = await import("./middleware");
    const res = await middleware(await reqWithSid(null, "/api/workspaces"));
    expect(res.status).toBe(401);
    const body = await res.json();
    expect(body.error).toBe("unauthenticated");
  });

  // #1556 — programmatic clients authenticate with an API key, not a session
  // cookie. The middleware must let them past the cookie gate so the route
  // handlers (getUser -> authenticateApiKey) can validate + authorize them.
  it("lets a Bearer API-key /api request past the cookie gate", async () => {
    const { middleware } = await import("./middleware");
    const res = await middleware(reqWithHeaders({ authorization: `Bearer ${API_KEY}` }));
    expect(res.status).toBe(200);
  });

  it("lets an X-API-Key /api request past the cookie gate", async () => {
    const { middleware } = await import("./middleware");
    const res = await middleware(reqWithHeaders({ "x-api-key": API_KEY }));
    expect(res.status).toBe(200);
  });

  it("still 401s an API-key request when API keys are disabled", async () => {
    process.env.OMNIA_AUTH_API_KEYS_ENABLED = "false";
    const { middleware } = await import("./middleware");
    const res = await middleware(reqWithHeaders({ authorization: `Bearer ${API_KEY}` }));
    expect(res.status).toBe(401);
  });

  it("does not treat a non-omnia_sk_ Bearer token as an API key", async () => {
    const { middleware } = await import("./middleware");
    const res = await middleware(reqWithHeaders({ authorization: "Bearer eyJhbGci.fake.jwt" }));
    expect(res.status).toBe(401);
  });

  it("does not bypass the gate for non-/api paths carrying an API key", async () => {
    const { middleware } = await import("./middleware");
    const res = await middleware(reqWithHeaders({ authorization: `Bearer ${API_KEY}` }, "/dashboard"));
    expect(res.status).toBe(307);
  });
});

import { describe, it, expect, vi, beforeEach } from "vitest";
import { MemorySessionStore } from "@/lib/auth/session-store/memory-store";

// cookieStore must be hoisted so the mock factory closure can close over it.
const { cookieStore } = vi.hoisted(() => ({
  cookieStore: new Map<string, string>(),
}));

// Instantiate the store after imports are available.
const store = new MemorySessionStore();

vi.mock("@/lib/auth/session-store", async () => {
  const actual = await vi.importActual<typeof import("@/lib/auth/session-store")>("@/lib/auth/session-store");
  return { ...actual, getSessionStore: () => store };
});

vi.mock("next/headers", () => ({
  cookies: async () => ({
    get: (n: string) => {
      const v = cookieStore.get(n);
      return v === undefined ? undefined : { name: n, value: v };
    },
    set: (name: string, value: string, options?: Record<string, unknown>) => {
      // iron-session's destroy() sets maxAge: 0; treat as delete.
      if (options && (options.maxAge === 0 || options.expires)) {
        cookieStore.delete(name);
      } else {
        cookieStore.set(name, value);
      }
    },
    delete: (n: string) => { cookieStore.delete(n); },
  }),
}));

vi.mock("@/lib/auth/oauth", () => ({
  buildEndSessionUrl: vi.fn(async (idt: string) => `https://idp/logout?id=${idt}`),
}));

beforeEach(() => {
  cookieStore.clear();
  process.env.OMNIA_AUTH_MODE = "oauth";
  process.env.OMNIA_SESSION_SECRET = "a".repeat(32);
  process.env.OMNIA_BASE_URL = "https://omnia.example";
});

describe("POST /api/auth/logout", () => {
  it("returns /login when no session exists", async () => {
    const { POST } = await import("./route");
    const res = await POST();
    const body = await res.json();
    expect(body.redirectUrl).toBe("/login");
  });

  it("returns the IdP end-session URL when the session has an idToken", async () => {
    const { saveUserToSession } = await import("@/lib/auth/session");
    await saveUserToSession(
      { id: "u1", username: "u1", groups: [], role: "viewer", provider: "oauth" },
      { provider: "azure", idToken: "it", refreshToken: "rt", expiresAt: 1 },
    );
    const { POST } = await import("./route");
    const res = await POST();
    const body = await res.json();
    expect(body.redirectUrl).toBe("https://idp/logout?id=it");
    expect(cookieStore.size).toBe(0); // cookie cleared
  });

  it("deletes the server record on logout", async () => {
    const { saveUserToSession, getSessionRecord } = await import("@/lib/auth/session");
    await saveUserToSession(
      { id: "u1", username: "u1", groups: [], role: "viewer", provider: "oauth" },
      { provider: "azure", idToken: "it", refreshToken: "rt", expiresAt: 1 },
    );
    expect(await getSessionRecord()).not.toBeNull();
    const { POST } = await import("./route");
    await POST();
    expect(await getSessionRecord()).toBeNull();
  });

  it("returns /login when session has no idToken", async () => {
    const { saveUserToSession } = await import("@/lib/auth/session");
    await saveUserToSession(
      { id: "u1", username: "u1", groups: [], role: "viewer", provider: "oauth" },
      { provider: "azure", refreshToken: "rt", expiresAt: 1 },
    );
    const { POST } = await import("./route");
    const res = await POST();
    const body = await res.json();
    expect(body.redirectUrl).toBe("/login");
  });

  it("returns /login when not in oauth mode", async () => {
    process.env.OMNIA_AUTH_MODE = "builtin";
    const { POST } = await import("./route");
    const res = await POST();
    const body = await res.json();
    expect(body.redirectUrl).toBe("/login");
  });
});

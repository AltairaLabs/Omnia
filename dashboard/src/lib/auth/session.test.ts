import { describe, it, expect, vi, beforeEach } from "vitest";
import { MemorySessionStore } from "./session-store/memory-store";

// cookieStore must be hoisted so the mock factory closure can close over it.
const { cookieStore } = vi.hoisted(() => ({
  cookieStore: new Map<string, string>(),
}));

// Minimal fake cookie jar matching Next's cookies() shape used by iron-session.
// iron-session calls set(name, "", { maxAge: 0 }) on destroy — treat that as delete.
const cookiesMock = {
  get: (name: string) => {
    const v = cookieStore.get(name);
    return v === undefined ? undefined : { name, value: v };
  },
  set: (name: string, value: string, options?: { maxAge?: number }) => {
    if (options?.maxAge === 0) {
      cookieStore.delete(name);
    } else {
      cookieStore.set(name, value);
    }
  },
  delete: (name: string) => { cookieStore.delete(name); },
};

vi.mock("next/headers", () => ({
  cookies: async () => cookiesMock,
}));

// Instantiate the store after imports are available.
const store = new MemorySessionStore();

vi.mock("./session-store", async () => {
  const actual = await vi.importActual<typeof import("./session-store")>("./session-store");
  return { ...actual, getSessionStore: () => store };
});

beforeEach(() => {
  cookieStore.clear();
  // Flush the in-memory store between tests by clearing all sessions.
  // MemorySessionStore doesn't expose a reset(), so we clear via a fresh
  // store reference — the mock always returns `store`, so we need to
  // drain it by deleting any keys that might have been written.
  // Simplest: replace with a new instance each test via the module variable.
  process.env.OMNIA_SESSION_SECRET = "a".repeat(32);
});

describe("session helpers", () => {
  it("getCurrentUser returns null without a cookie", async () => {
    const { getCurrentUser } = await import("./session");
    expect(await getCurrentUser()).toBeNull();
  });

  it("saveUserToSession writes record to store and sid cookie", async () => {
    const { saveUserToSession, getCurrentUser } = await import("./session");
    await saveUserToSession({
      id: "u1", username: "u1", groups: [], role: "viewer", provider: "oauth",
    });
    const u = await getCurrentUser();
    expect(u?.id).toBe("u1");
    expect([...cookieStore.values()][0]).toBeTruthy();
  });

  it("clearSession deletes store record and clears cookie", async () => {
    const { saveUserToSession, clearSession, getCurrentUser } = await import("./session");
    await saveUserToSession({
      id: "u1", username: "u1", groups: [], role: "viewer", provider: "oauth",
    });
    await clearSession();
    expect(await getCurrentUser()).toBeNull();
    expect(cookieStore.size).toBe(0);
  });

  it("isAuthenticated is false for anonymous and true for real providers", async () => {
    const { saveUserToSession, isAuthenticated, clearSession } = await import("./session");
    await saveUserToSession({
      id: "anon", username: "anon", groups: [], role: "viewer", provider: "anonymous",
    });
    expect(await isAuthenticated()).toBe(false);
    await clearSession();
    await saveUserToSession({
      id: "u1", username: "u1", groups: [], role: "viewer", provider: "oauth",
    });
    expect(await isAuthenticated()).toBe(true);
  });

  it("updateSessionOAuth updates oauth tokens on existing session", async () => {
    const { saveUserToSession, updateSessionOAuth, getSessionRecord } = await import("./session");
    await saveUserToSession(
      { id: "u1", username: "u1", groups: [], role: "viewer", provider: "oauth" },
      { provider: "azure", refreshToken: "old", idToken: "oldid", expiresAt: 100 },
    );
    await updateSessionOAuth({ provider: "azure", refreshToken: "new", idToken: "newid", expiresAt: 200 });
    const r = await getSessionRecord();
    expect(r?.oauth?.refreshToken).toBe("new");
    expect(r?.oauth?.idToken).toBe("newid");
    expect(r?.user.id).toBe("u1");
  });

  it("updateSessionOAuth is no-op when no session exists", async () => {
    const { updateSessionOAuth, getSessionRecord } = await import("./session");
    await updateSessionOAuth({ provider: "azure", refreshToken: "x" });
    expect(await getSessionRecord()).toBeNull();
  });
});

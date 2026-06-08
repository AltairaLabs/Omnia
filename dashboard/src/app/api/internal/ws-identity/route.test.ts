import { describe, it, expect, vi, beforeEach } from "vitest";
import type { User } from "@/lib/auth/types";

const { getCurrentUser } = vi.hoisted(() => ({
  getCurrentUser: vi.fn<() => Promise<User | null>>(),
}));

vi.mock("@/lib/auth/session", () => ({ getCurrentUser }));

function user(overrides: Partial<User> = {}): User {
  return {
    id: "stable-user-123",
    username: "jdoe",
    groups: [],
    role: "viewer",
    provider: "oauth",
    ...overrides,
  };
}

describe("GET /api/internal/ws-identity", () => {
  beforeEach(() => {
    getCurrentUser.mockReset();
  });

  it("returns the raw user id for an authenticated user", async () => {
    getCurrentUser.mockResolvedValue(user());
    const { GET } = await import("./route");
    const res = await GET();
    expect(await res.json()).toEqual({ userId: "stable-user-123" });
  });

  it("returns null when no user resolves (anonymous / no session)", async () => {
    getCurrentUser.mockResolvedValue(null);
    const { GET } = await import("./route");
    const res = await GET();
    expect(await res.json()).toEqual({ userId: null });
  });

  it("returns null for an anonymous-provider user (never scopes to a pseudonym)", async () => {
    getCurrentUser.mockResolvedValue(user({ id: "anonymous", provider: "anonymous" }));
    const { GET } = await import("./route");
    const res = await GET();
    expect(await res.json()).toEqual({ userId: null });
  });
});

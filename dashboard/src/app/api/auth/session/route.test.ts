import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("@/lib/auth", () => ({
  getUser: vi.fn(),
}));

beforeEach(() => {
  vi.resetModules();
});

describe("GET /api/auth/session", () => {
  it("returns 200 authenticated:true for an oauth user", async () => {
    const { getUser } = await import("@/lib/auth");
    vi.mocked(getUser).mockResolvedValue({
      id: "u1",
      username: "alice",
      groups: [],
      role: "viewer",
      provider: "oauth",
    });
    const { GET } = await import("./route");
    const res = await GET();
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body).toEqual({ authenticated: true });
  });

  it("returns 200 authenticated:true for a builtin user", async () => {
    const { getUser } = await import("@/lib/auth");
    vi.mocked(getUser).mockResolvedValue({
      id: "u2",
      username: "bob",
      groups: [],
      role: "editor",
      provider: "builtin",
    });
    const { GET } = await import("./route");
    const res = await GET();
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body).toEqual({ authenticated: true });
  });

  it("returns 200 authenticated:true for a proxy user", async () => {
    const { getUser } = await import("@/lib/auth");
    vi.mocked(getUser).mockResolvedValue({
      id: "u3",
      username: "carol",
      groups: [],
      role: "admin",
      provider: "proxy",
    });
    const { GET } = await import("./route");
    const res = await GET();
    expect(res.status).toBe(200);
    const body = await res.json();
    expect(body).toEqual({ authenticated: true });
  });

  it("returns 401 authenticated:false for an anonymous user", async () => {
    const { getUser } = await import("@/lib/auth");
    vi.mocked(getUser).mockResolvedValue({
      id: "anonymous",
      username: "anonymous",
      groups: [],
      role: "viewer",
      provider: "anonymous",
    });
    const { GET } = await import("./route");
    const res = await GET();
    expect(res.status).toBe(401);
    const body = await res.json();
    expect(body).toEqual({ authenticated: false });
  });
});

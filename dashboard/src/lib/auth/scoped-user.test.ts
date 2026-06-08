import { describe, it, expect, vi, afterEach } from "vitest";
import { resolveScopedUserId } from "./scoped-user";
import type { User } from "./types";

function authUser(id = "session-user"): User {
  return { id, provider: "oauth", username: "u", groups: [], role: "viewer" };
}

const anonUser: User = {
  id: "anonymous",
  provider: "anonymous",
  username: "anonymous",
  groups: [],
  role: "viewer",
};

describe("resolveScopedUserId", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("scopes an authenticated user to their session id, ignoring the query param", () => {
    const params = new URLSearchParams("userId=someone-else");
    expect(resolveScopedUserId(params, authUser("alice"))).toBe("alice");
  });

  it("returns the session id even when no userId is supplied", () => {
    expect(resolveScopedUserId(new URLSearchParams(), authUser("alice"))).toBe("alice");
  });

  it("warns when an authenticated request carries a mismatched userId", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    resolveScopedUserId(new URLSearchParams("userId=victim"), authUser("alice"));
    expect(warn).toHaveBeenCalledOnce();
  });

  it("does not warn when the supplied userId matches the session user", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    resolveScopedUserId(new URLSearchParams("userId=alice"), authUser("alice"));
    expect(warn).not.toHaveBeenCalled();
  });

  it("scopes an anonymous user to their client-supplied device id", () => {
    expect(resolveScopedUserId(new URLSearchParams("userId=device-1"), anonUser)).toBe("device-1");
  });

  it("returns null for an anonymous user with no device id", () => {
    expect(resolveScopedUserId(new URLSearchParams(), anonUser)).toBeNull();
  });
});

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { NextRequest, NextResponse } from "next/server";
import { middleware } from "./middleware";

function makeRequest(
  path: string,
  opts: { cookie?: string; search?: string } = {},
): NextRequest {
  const url = `http://localhost:3000${path}${opts.search ?? ""}`;
  const headers = new Headers();
  if (opts.cookie) headers.set("cookie", opts.cookie);
  return new NextRequest(url, { headers });
}

describe("dashboard auth middleware", () => {
  const originalMode = process.env.OMNIA_AUTH_MODE;
  const originalCookieName = process.env.OMNIA_SESSION_COOKIE_NAME;

  beforeEach(() => {
    delete process.env.OMNIA_AUTH_MODE;
    delete process.env.OMNIA_SESSION_COOKIE_NAME;
  });

  afterEach(() => {
    if (originalMode === undefined) {
      delete process.env.OMNIA_AUTH_MODE;
    } else {
      process.env.OMNIA_AUTH_MODE = originalMode;
    }
    if (originalCookieName === undefined) {
      delete process.env.OMNIA_SESSION_COOKIE_NAME;
    } else {
      process.env.OMNIA_SESSION_COOKIE_NAME = originalCookieName;
    }
    vi.restoreAllMocks();
  });

  it("passes everything through when mode is anonymous", async () => {
    process.env.OMNIA_AUTH_MODE = "anonymous";
    const nextSpy = vi.spyOn(NextResponse, "next");
    await middleware(makeRequest("/some/protected/path"));
    expect(nextSpy).toHaveBeenCalled();
  });

  it("passes everything through when OMNIA_AUTH_MODE is unset (defaults to anonymous)", async () => {
    const nextSpy = vi.spyOn(NextResponse, "next");
    await middleware(makeRequest("/"));
    expect(nextSpy).toHaveBeenCalled();
  });

  it.each([
    ["/login", "login page"],
    ["/login?error=foo", "login page with query"],
    ["/api/auth/login", "oauth login endpoint"],
    ["/api/auth/callback", "oauth callback endpoint"],
    ["/api/auth/logout", "logout endpoint"],
    ["/api/auth/refresh", "token refresh endpoint"],
    ["/api/auth/builtin/signup", "builtin signup"],
    ["/api/auth/builtin/forgot-password", "builtin forgot-password"],
    ["/api/health", "health endpoint"],
    ["/api/config", "config endpoint (login page depends on it)"],
    ["/api/license", "license endpoint"],
    ["/_next/static/chunks/foo.js", "Next.js static asset"],
    ["/favicon.ico", "favicon"],
  ])("allows %s unauthenticated in oauth mode (%s)", async (path) => {
    process.env.OMNIA_AUTH_MODE = "oauth";
    const nextSpy = vi.spyOn(NextResponse, "next");
    await middleware(makeRequest(path));
    expect(nextSpy).toHaveBeenCalled();
  });

  it("redirects unauthenticated page requests to /login with returnTo", async () => {
    process.env.OMNIA_AUTH_MODE = "oauth";
    const resp = await middleware(makeRequest("/sessions/abc?tab=replay"));
    expect(resp.status).toBe(307);
    const location = resp.headers.get("location")!;
    const locUrl = new URL(location);
    expect(locUrl.pathname).toBe("/login");
    expect(locUrl.searchParams.get("returnTo")).toBe("/sessions/abc?tab=replay");
  });

  it("returns 401 JSON for unauthenticated API requests", async () => {
    process.env.OMNIA_AUTH_MODE = "oauth";
    const resp = await middleware(makeRequest("/api/workspaces/foo/skills"));
    expect(resp.status).toBe(401);
    const body = await resp.json();
    expect(body).toEqual({ error: "unauthenticated" });
  });

  it("allows authenticated requests when the session cookie is present", async () => {
    process.env.OMNIA_AUTH_MODE = "oauth";
    const nextSpy = vi.spyOn(NextResponse, "next");
    await middleware(
      makeRequest("/sessions/abc", { cookie: "omnia_session=opaque" }),
    );
    expect(nextSpy).toHaveBeenCalled();
  });

  it("respects a custom OMNIA_SESSION_COOKIE_NAME", async () => {
    process.env.OMNIA_AUTH_MODE = "oauth";
    process.env.OMNIA_SESSION_COOKIE_NAME = "acme_auth";
    const nextSpy = vi.spyOn(NextResponse, "next");
    await middleware(makeRequest("/sessions/abc", { cookie: "acme_auth=opaque" }));
    expect(nextSpy).toHaveBeenCalled();
  });

  it("enforces auth in builtin mode the same way", async () => {
    process.env.OMNIA_AUTH_MODE = "builtin";
    const resp = await middleware(makeRequest("/"));
    expect(resp.status).toBe(307);
    const location = new URL(resp.headers.get("location")!);
    expect(location.pathname).toBe("/login");
  });

  it("enforces auth in proxy mode the same way", async () => {
    process.env.OMNIA_AUTH_MODE = "proxy";
    const resp = await middleware(makeRequest("/"));
    expect(resp.status).toBe(307);
    const location = new URL(resp.headers.get("location")!);
    expect(location.pathname).toBe("/login");
  });
});

/**
 * Tests for Next.js auth middleware.
 *
 * Covers the four branches: public-route bypass, anonymous mode pass-through,
 * proxy mode API 401 on missing header, and oauth/builtin redirect-vs-401 for
 * page vs API routes.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { NextRequest } from "next/server";
import { middleware } from "./middleware";

function makeRequest(
  path: string,
  init: { cookies?: Record<string, string>; headers?: Record<string, string> } = {}
): NextRequest {
  const url = `http://localhost:3000${path}`;
  const headers = new Headers(init.headers);
  if (init.cookies) {
    headers.set(
      "cookie",
      Object.entries(init.cookies)
        .map(([k, v]) => `${k}=${v}`)
        .join("; ")
    );
  }
  return new NextRequest(new Request(url, { headers }));
}

describe("auth middleware", () => {
  const originalEnv = process.env;

  beforeEach(() => {
    process.env = { ...originalEnv };
    delete process.env.OMNIA_AUTH_MODE;
    delete process.env.OMNIA_AUTH_PROXY_HEADER_USER;
    delete process.env.OMNIA_SESSION_COOKIE_NAME;
  });

  afterEach(() => {
    process.env = originalEnv;
    vi.restoreAllMocks();
  });

  describe("public routes", () => {
    it.each([
      "/login",
      "/signup",
      "/forgot-password",
      "/reset-password",
      "/verify-email",
      "/api/health",
      "/api/auth/login",
      "/api/auth/callback",
      "/api/auth/logout",
      "/api/auth/builtin/login",
      "/favicon.ico",
      "/_next/static/chunks/main.js",
      "/logo.svg",
      "/logo-dark.svg",
    ])("bypasses auth for %s even when mode=oauth", (path) => {
      process.env.OMNIA_AUTH_MODE = "oauth";
      const res = middleware(makeRequest(path));
      // NextResponse.next() has no status redirect and no body — not a redirect.
      expect(res.status).toBe(200);
      expect(res.headers.get("location")).toBeNull();
    });
  });

  describe("anonymous mode", () => {
    it("allows all protected routes through", () => {
      process.env.OMNIA_AUTH_MODE = "anonymous";
      const res = middleware(makeRequest("/agents"));
      expect(res.status).toBe(200);
      expect(res.headers.get("location")).toBeNull();
    });

    it("treats unset mode as anonymous", () => {
      const res = middleware(makeRequest("/agents"));
      expect(res.status).toBe(200);
    });
  });

  describe("proxy mode", () => {
    beforeEach(() => {
      process.env.OMNIA_AUTH_MODE = "proxy";
    });

    it("returns 401 JSON for API route without user header", async () => {
      const res = middleware(makeRequest("/api/stats"));
      expect(res.status).toBe(401);
      await expect(res.json()).resolves.toEqual({
        error: "Authentication required",
      });
    });

    it("allows API route when user header is present", () => {
      const res = middleware(
        makeRequest("/api/stats", { headers: { "X-Forwarded-User": "alice" } })
      );
      expect(res.status).toBe(200);
    });

    it("honors custom header name from env", () => {
      process.env.OMNIA_AUTH_PROXY_HEADER_USER = "X-Auth-User";
      const res = middleware(
        makeRequest("/api/stats", { headers: { "X-Auth-User": "alice" } })
      );
      expect(res.status).toBe(200);
    });

    it("allows page routes without header (app renders anonymous UI)", () => {
      const res = middleware(makeRequest("/agents"));
      expect(res.status).toBe(200);
    });

    it("never 401s the health endpoint", () => {
      const res = middleware(makeRequest("/api/health"));
      expect(res.status).toBe(200);
    });
  });

  describe.each(["oauth", "builtin"] as const)("%s mode", (mode) => {
    beforeEach(() => {
      process.env.OMNIA_AUTH_MODE = mode;
    });

    it("redirects unauthenticated page requests to /login with returnTo", () => {
      const res = middleware(makeRequest("/agents"));
      expect(res.status).toBe(307);
      const location = res.headers.get("location");
      expect(location).not.toBeNull();
      const redirectUrl = new URL(location as string);
      expect(redirectUrl.pathname).toBe("/login");
      expect(redirectUrl.searchParams.get("returnTo")).toBe("/agents");
    });

    it("returns 401 JSON for unauthenticated API requests", async () => {
      const res = middleware(makeRequest("/api/stats"));
      expect(res.status).toBe(401);
      await expect(res.json()).resolves.toEqual({
        error: "Authentication required",
        loginRequired: true,
      });
    });

    it("allows requests that carry the session cookie", () => {
      const res = middleware(
        makeRequest("/agents", { cookies: { omnia_session: "opaque-value" } })
      );
      expect(res.status).toBe(200);
      expect(res.headers.get("location")).toBeNull();
    });

    it("honors custom session cookie name from env", () => {
      process.env.OMNIA_SESSION_COOKIE_NAME = "custom_sess";
      const res = middleware(
        makeRequest("/agents", { cookies: { custom_sess: "value" } })
      );
      expect(res.status).toBe(200);
    });

    it("redirects the root path preserving returnTo=/", () => {
      const res = middleware(makeRequest("/"));
      expect(res.status).toBe(307);
      const redirectUrl = new URL(res.headers.get("location") as string);
      expect(redirectUrl.searchParams.get("returnTo")).toBe("/");
    });
  });
});

describe("middleware config", () => {
  it("excludes static assets from the matcher", async () => {
    const mod = await import("./middleware");
    expect(mod.config.matcher).toEqual([
      "/((?!_next/static|_next/image|favicon.ico).*)",
    ]);
  });
});

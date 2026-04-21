import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { NextRequest, NextResponse } from "next/server";
import { sealData } from "iron-session";
import { middleware } from "./middleware";
import type { User } from "./lib/auth/types";

// iron-session requires a password of ≥ 32 characters.
const SESSION_SECRET = "test-secret-at-least-32-characters-long-ok";
const COOKIE_NAME = "omnia_session";

/** Build a real iron-session cookie value that the middleware can decrypt. */
async function sealSession(data: { user?: User }): Promise<string> {
  return sealData(data, {
    password: SESSION_SECRET,
    ttl: 0,
  });
}

function makeRequest(
  path: string,
  opts: { cookie?: string; search?: string } = {},
): NextRequest {
  const url = `http://localhost:3000${path}${opts.search ?? ""}`;
  const headers = new Headers();
  if (opts.cookie) headers.set("cookie", opts.cookie);
  return new NextRequest(url, { headers });
}

const OAUTH_USER: User = {
  id: "u1",
  username: "alice",
  email: "alice@example.com",
  groups: [],
  role: "viewer",
  provider: "oauth",
};

const BUILTIN_USER: User = { ...OAUTH_USER, provider: "builtin" };
const ANONYMOUS_USER: User = { ...OAUTH_USER, id: "anonymous", username: "anonymous", provider: "anonymous" };

describe("dashboard auth middleware", () => {
  const originalMode = process.env.OMNIA_AUTH_MODE;
  const originalCookieName = process.env.OMNIA_SESSION_COOKIE_NAME;
  const originalSessionSecret = process.env.OMNIA_SESSION_SECRET;

  beforeEach(() => {
    delete process.env.OMNIA_AUTH_MODE;
    delete process.env.OMNIA_SESSION_COOKIE_NAME;
    process.env.OMNIA_SESSION_SECRET = SESSION_SECRET;
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
    if (originalSessionSecret === undefined) {
      delete process.env.OMNIA_SESSION_SECRET;
    } else {
      process.env.OMNIA_SESSION_SECRET = originalSessionSecret;
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

  describe("oauth mode", () => {
    beforeEach(() => {
      process.env.OMNIA_AUTH_MODE = "oauth";
    });

    it("redirects unauthenticated page requests to /login with returnTo", async () => {
      const resp = await middleware(makeRequest("/sessions/abc?tab=replay"));
      expect(resp.status).toBe(307);
      const locUrl = new URL(resp.headers.get("location")!);
      expect(locUrl.pathname).toBe("/login");
      expect(locUrl.searchParams.get("returnTo")).toBe("/sessions/abc?tab=replay");
    });

    it("returns 401 JSON for unauthenticated API requests", async () => {
      const resp = await middleware(makeRequest("/api/workspaces/foo/skills"));
      expect(resp.status).toBe(401);
      expect(await resp.json()).toEqual({ error: "unauthenticated" });
    });

    it("allows page requests carrying a valid oauth session", async () => {
      const sealed = await sealSession({ user: OAUTH_USER });
      const nextSpy = vi.spyOn(NextResponse, "next");
      await middleware(makeRequest("/sessions/abc", { cookie: `${COOKIE_NAME}=${sealed}` }));
      expect(nextSpy).toHaveBeenCalled();
    });

    it("rejects page requests with a bogus cookie and clears it", async () => {
      const resp = await middleware(
        makeRequest("/sessions/abc", { cookie: `${COOKIE_NAME}=not-a-real-iron-session` }),
      );
      expect(resp.status).toBe(307);
      expect(new URL(resp.headers.get("location")!).pathname).toBe("/login");
      // Set-Cookie should clear the invalid cookie on the way back.
      const setCookie = resp.headers.get("set-cookie")!;
      expect(setCookie).toMatch(/omnia_session=;/);
      expect(setCookie).toMatch(/Max-Age=0|Expires=/i);
    });

    it("rejects API requests with a bogus cookie and clears it", async () => {
      const resp = await middleware(
        makeRequest("/api/workspaces", { cookie: `${COOKIE_NAME}=bogus` }),
      );
      expect(resp.status).toBe(401);
      expect(await resp.json()).toEqual({ error: "unauthenticated" });
      expect(resp.headers.get("set-cookie")).toMatch(/omnia_session=;/);
    });

    it("rejects a session whose provider is anonymous", async () => {
      const sealed = await sealSession({ user: ANONYMOUS_USER });
      const resp = await middleware(makeRequest("/", { cookie: `${COOKIE_NAME}=${sealed}` }));
      expect(resp.status).toBe(307);
    });

    it("rejects a session whose provider is builtin (mode mismatch)", async () => {
      const sealed = await sealSession({ user: BUILTIN_USER });
      const resp = await middleware(makeRequest("/", { cookie: `${COOKIE_NAME}=${sealed}` }));
      expect(resp.status).toBe(307);
    });

    it("rejects a session with no user object", async () => {
      const sealed = await sealSession({});
      const resp = await middleware(makeRequest("/", { cookie: `${COOKIE_NAME}=${sealed}` }));
      expect(resp.status).toBe(307);
    });

    it("respects a custom OMNIA_SESSION_COOKIE_NAME", async () => {
      process.env.OMNIA_SESSION_COOKIE_NAME = "acme_auth";
      const sealed = await sealSession({ user: OAUTH_USER });
      const nextSpy = vi.spyOn(NextResponse, "next");
      await middleware(makeRequest("/sessions/abc", { cookie: `acme_auth=${sealed}` }));
      expect(nextSpy).toHaveBeenCalled();
    });
  });

  describe("builtin mode", () => {
    beforeEach(() => {
      process.env.OMNIA_AUTH_MODE = "builtin";
    });

    it("redirects unauthenticated page requests", async () => {
      const resp = await middleware(makeRequest("/"));
      expect(resp.status).toBe(307);
      expect(new URL(resp.headers.get("location")!).pathname).toBe("/login");
    });

    it("allows a valid builtin session", async () => {
      const sealed = await sealSession({ user: BUILTIN_USER });
      const nextSpy = vi.spyOn(NextResponse, "next");
      await middleware(makeRequest("/", { cookie: `${COOKIE_NAME}=${sealed}` }));
      expect(nextSpy).toHaveBeenCalled();
    });

    it("rejects an oauth-provider session in builtin mode", async () => {
      const sealed = await sealSession({ user: OAUTH_USER });
      const resp = await middleware(makeRequest("/", { cookie: `${COOKIE_NAME}=${sealed}` }));
      expect(resp.status).toBe(307);
    });
  });

  describe("proxy mode", () => {
    beforeEach(() => {
      process.env.OMNIA_AUTH_MODE = "proxy";
    });

    it("redirects page requests without any session cookie", async () => {
      const resp = await middleware(makeRequest("/"));
      expect(resp.status).toBe(307);
    });

    it("lets through page requests that carry *any* session cookie (presence check)", async () => {
      // Proxy deployments mint the session on the first authenticated hit;
      // middleware shouldn't gate that with full decryption or the cold
      // start round-trips through /login unnecessarily.
      const nextSpy = vi.spyOn(NextResponse, "next");
      await middleware(makeRequest("/", { cookie: `${COOKIE_NAME}=whatever` }));
      expect(nextSpy).toHaveBeenCalled();
    });
  });

  describe("security response headers (H-1)", () => {
    it("applies HSTS / CSP / X-Frame-Options / nosniff / Referrer-Policy / Permissions-Policy on pass-through", async () => {
      process.env.OMNIA_AUTH_MODE = "anonymous";
      const resp = await middleware(makeRequest("/"));
      expect(resp.headers.get("Strict-Transport-Security")).toContain("max-age=");
      expect(resp.headers.get("Strict-Transport-Security")).toContain("includeSubDomains");
      expect(resp.headers.get("Content-Security-Policy")).toContain("default-src 'self'");
      expect(resp.headers.get("Content-Security-Policy")).toContain("frame-ancestors 'none'");
      expect(resp.headers.get("X-Frame-Options")).toBe("DENY");
      expect(resp.headers.get("X-Content-Type-Options")).toBe("nosniff");
      expect(resp.headers.get("Referrer-Policy")).toBe("strict-origin-when-cross-origin");
      expect(resp.headers.get("Permissions-Policy")).toContain("camera=()");
    });

    it("applies security headers on a /login redirect (unauthenticated path response)", async () => {
      process.env.OMNIA_AUTH_MODE = "oauth";
      const resp = await middleware(makeRequest("/sessions/abc"));
      expect(resp.status).toBe(307);
      expect(resp.headers.get("Strict-Transport-Security")).toContain("max-age=");
      expect(resp.headers.get("Content-Security-Policy")).toContain("default-src 'self'");
    });

    it("applies security headers on 401 API responses", async () => {
      process.env.OMNIA_AUTH_MODE = "oauth";
      const resp = await middleware(makeRequest("/api/workspaces"));
      expect(resp.status).toBe(401);
      expect(resp.headers.get("X-Frame-Options")).toBe("DENY");
      expect(resp.headers.get("X-Content-Type-Options")).toBe("nosniff");
    });

    it("respects OMNIA_CSP_POLICY override", async () => {
      const originalPolicy = process.env.OMNIA_CSP_POLICY;
      process.env.OMNIA_AUTH_MODE = "anonymous";
      process.env.OMNIA_CSP_POLICY = "default-src 'self'; script-src 'self'";
      try {
        const resp = await middleware(makeRequest("/"));
        expect(resp.headers.get("Content-Security-Policy")).toBe(
          "default-src 'self'; script-src 'self'",
        );
      } finally {
        if (originalPolicy === undefined) delete process.env.OMNIA_CSP_POLICY;
        else process.env.OMNIA_CSP_POLICY = originalPolicy;
      }
    });
  });

  describe("cleared-session cookie security attributes (H-2)", () => {
    beforeEach(() => {
      process.env.OMNIA_AUTH_MODE = "oauth";
    });

    it("sets HttpOnly + SameSite + Path on the cleared cookie", async () => {
      const resp = await middleware(
        makeRequest("/sessions/abc", { cookie: `${COOKIE_NAME}=bogus` }),
      );
      const setCookie = resp.headers.get("set-cookie")!;
      // Reflect the original cookie's security posture on the clearing
      // Set-Cookie header, otherwise a MITM can observe that the session
      // was cleared over plaintext and JS can read the (empty) cookie.
      expect(setCookie).toMatch(/HttpOnly/i);
      expect(setCookie).toMatch(/SameSite=Lax/i);
      expect(setCookie).toMatch(/Path=\//i);
      expect(setCookie).toMatch(/Max-Age=0|Expires=/i);
    });

    it("adds Secure in production NODE_ENV", async () => {
      const originalEnv = process.env.NODE_ENV;
      // Next.js typings make NODE_ENV readonly; mutate via Object.assign to
      // keep vitest happy while still setting the runtime value.
      Object.assign(process.env, { NODE_ENV: "production" });
      try {
        const resp = await middleware(
          makeRequest("/sessions/abc", { cookie: `${COOKIE_NAME}=bogus` }),
        );
        expect(resp.headers.get("set-cookie")!).toMatch(/Secure/i);
      } finally {
        Object.assign(process.env, { NODE_ENV: originalEnv });
      }
    });
  });
});

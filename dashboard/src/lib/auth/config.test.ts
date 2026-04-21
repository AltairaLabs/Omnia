/**
 * Tests for auth config — covers default values and env-var overrides,
 * including the OMNIA_SESSION_STORE and OMNIA_SESSION_PKCE_TTL knobs
 * added in Task 6.
 */
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { writeFileSync, mkdtempSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { getAuthConfig, isAuthEnabled } from "./config";

describe("getAuthConfig defaults", () => {
  beforeEach(() => {
    // Clear relevant env vars so defaults apply
    vi.unstubAllEnvs();
  });

  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("returns anonymous mode by default", () => {
    const cfg = getAuthConfig();
    expect(cfg.mode).toBe("anonymous");
  });

  it("returns default session cookie name", () => {
    const cfg = getAuthConfig();
    expect(cfg.session.cookieName).toBe("omnia_session");
  });

  it("returns default session TTL of 86400", () => {
    const cfg = getAuthConfig();
    expect(cfg.session.ttl).toBe(86400);
  });

  it("defaults storeBackend to memory", () => {
    const cfg = getAuthConfig();
    expect(cfg.session.storeBackend).toBe("memory");
  });

  it("defaults pkceTtl to 300 seconds", () => {
    const cfg = getAuthConfig();
    expect(cfg.session.pkceTtl).toBe(300);
  });

  it("returns default base URL", () => {
    const cfg = getAuthConfig();
    expect(cfg.baseUrl).toBe("http://localhost:3000");
  });

  it("defaults proxy autoSignup to true", () => {
    const cfg = getAuthConfig();
    expect(cfg.proxy.autoSignup).toBe(true);
  });

  it("returns default roleMapping with empty arrays", () => {
    const cfg = getAuthConfig();
    expect(cfg.roleMapping.admin).toEqual([]);
    expect(cfg.roleMapping.editor).toEqual([]);
  });

  it("returns default OAuth provider as generic", () => {
    const cfg = getAuthConfig();
    expect(cfg.oauth.provider).toBe("generic");
  });
});

describe("getAuthConfig env-var overrides", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("reads OMNIA_AUTH_MODE", () => {
    vi.stubEnv("OMNIA_AUTH_MODE", "oauth");
    expect(getAuthConfig().mode).toBe("oauth");
  });

  it("reads OMNIA_SESSION_STORE=redis → storeBackend redis", () => {
    vi.stubEnv("OMNIA_SESSION_STORE", "redis");
    expect(getAuthConfig().session.storeBackend).toBe("redis");
  });

  it("reads OMNIA_SESSION_STORE=memory → storeBackend memory", () => {
    vi.stubEnv("OMNIA_SESSION_STORE", "memory");
    expect(getAuthConfig().session.storeBackend).toBe("memory");
  });

  it("reads OMNIA_SESSION_PKCE_TTL", () => {
    vi.stubEnv("OMNIA_SESSION_PKCE_TTL", "600");
    expect(getAuthConfig().session.pkceTtl).toBe(600);
  });

  it("reads OMNIA_SESSION_COOKIE_NAME", () => {
    vi.stubEnv("OMNIA_SESSION_COOKIE_NAME", "my_cookie");
    expect(getAuthConfig().session.cookieName).toBe("my_cookie");
  });

  it("reads OMNIA_SESSION_TTL", () => {
    vi.stubEnv("OMNIA_SESSION_TTL", "3600");
    expect(getAuthConfig().session.ttl).toBe(3600);
  });

  it("reads OMNIA_SESSION_SECRET", () => {
    vi.stubEnv("OMNIA_SESSION_SECRET", "super-secret-value-that-is-long-enough");
    expect(getAuthConfig().session.secret).toBe("super-secret-value-that-is-long-enough");
  });

  it("reads OMNIA_BASE_URL", () => {
    vi.stubEnv("OMNIA_BASE_URL", "https://example.com");
    expect(getAuthConfig().baseUrl).toBe("https://example.com");
  });

  it("reads OMNIA_AUTH_PROXY_AUTO_SIGNUP=false", () => {
    vi.stubEnv("OMNIA_AUTH_PROXY_AUTO_SIGNUP", "false");
    expect(getAuthConfig().proxy.autoSignup).toBe(false);
  });

  it("reads OMNIA_AUTH_ROLE_ADMIN_GROUPS", () => {
    vi.stubEnv("OMNIA_AUTH_ROLE_ADMIN_GROUPS", "admins,superusers");
    expect(getAuthConfig().roleMapping.admin).toEqual(["admins", "superusers"]);
  });

  it("reads OMNIA_AUTH_ROLE_EDITOR_GROUPS", () => {
    vi.stubEnv("OMNIA_AUTH_ROLE_EDITOR_GROUPS", "editors");
    expect(getAuthConfig().roleMapping.editor).toEqual(["editors"]);
  });

  it("reads OMNIA_OAUTH_CLIENT_SECRET", () => {
    vi.stubEnv("OMNIA_OAUTH_CLIENT_SECRET", "client-secret-123");
    expect(getAuthConfig().oauth.clientSecret).toBe("client-secret-123");
  });

  it("reads OAuth client secret from OMNIA_OAUTH_CLIENT_SECRET_FILE", () => {
    const dir = mkdtempSync(join(tmpdir(), "omnia-test-"));
    const secretFile = join(dir, "client-secret");
    writeFileSync(secretFile, "file-secret-value\n");
    vi.stubEnv("OMNIA_OAUTH_CLIENT_SECRET_FILE", secretFile);
    expect(getAuthConfig().oauth.clientSecret).toBe("file-secret-value");
  });

  it("returns empty string when OAuth secret file does not exist", () => {
    vi.stubEnv("OMNIA_OAUTH_CLIENT_SECRET_FILE", "/nonexistent/path/secret");
    expect(getAuthConfig().oauth.clientSecret).toBe("");
  });

  it("logs warning in production when session secret is missing", () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    vi.stubEnv("NODE_ENV", "production");
    getAuthConfig();
    expect(warn).toHaveBeenCalledWith(expect.stringContaining("OMNIA_SESSION_SECRET"));
    warn.mockRestore();
  });

  it("reads OMNIA_OAUTH_SCOPES as comma-separated list", () => {
    vi.stubEnv("OMNIA_OAUTH_SCOPES", "openid,email");
    expect(getAuthConfig().oauth.scopes).toEqual(["openid", "email"]);
  });

  it("reads OMNIA_AUTH_ANONYMOUS_ROLE", () => {
    vi.stubEnv("OMNIA_AUTH_ANONYMOUS_ROLE", "admin");
    expect(getAuthConfig().anonymous.role).toBe("admin");
  });
});

describe("isAuthEnabled", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("returns false when mode is anonymous (default)", () => {
    expect(isAuthEnabled()).toBe(false);
  });

  it("returns true when mode is oauth", () => {
    vi.stubEnv("OMNIA_AUTH_MODE", "oauth");
    expect(isAuthEnabled()).toBe(true);
  });

  it("returns true when mode is proxy", () => {
    vi.stubEnv("OMNIA_AUTH_MODE", "proxy");
    expect(isAuthEnabled()).toBe(true);
  });

  it("returns true when mode is builtin", () => {
    vi.stubEnv("OMNIA_AUTH_MODE", "builtin");
    expect(isAuthEnabled()).toBe(true);
  });
});

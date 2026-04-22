import { describe, it, expect, vi, beforeEach } from "vitest";
import type { PKCEData } from "./types";

// Mock the openid-client module so we can observe what URL
// exchangeCodeForTokens hands to authorizationCodeGrant. The rest of
// the module (discovery, refresh, userinfo, end-session) sits on the
// happy-path cache and is exercised by the callback-route integration
// tests; this file exists specifically to pin the issue-#948 contract.
vi.mock("openid-client", async () => {
  const actual = await vi.importActual<typeof import("openid-client")>("openid-client");
  return {
    ...actual,
    discovery: vi.fn(async () => ({
      serverMetadata: () => ({}),
    })),
    authorizationCodeGrant: vi.fn(async () => ({
      access_token: "at",
      refresh_token: "rt",
      id_token: "it",
      expires_at: 0,
    })),
    // PKCE primitives: the real implementations depend on oauth4webapi's
    // Uint8Array handling that vitest's JSDOM env doesn't reliably
    // support. Mock to stable strings so generatePKCE round-trips
    // without touching node:crypto from a browser-like runtime.
    randomPKCECodeVerifier: () => "test-verifier",
    calculatePKCECodeChallenge: async () => "test-challenge",
    randomState: () => "test-state",
  };
});

describe("exchangeCodeForTokens (issue #948)", () => {
  const pkce: PKCEData = {
    codeVerifier: "v",
    codeChallenge: "c",
    state: "state-abc",
  };

  beforeEach(async () => {
    process.env.OMNIA_AUTH_MODE = "oauth";
    process.env.OMNIA_BASE_URL = "https://omnia.example";
    process.env.OMNIA_OAUTH_PROVIDER = "google";
    process.env.OMNIA_OAUTH_ISSUER_URL = "https://accounts.google.com";
    process.env.OMNIA_OAUTH_CLIENT_ID = "client-id";
    process.env.OMNIA_OAUTH_CLIENT_SECRET = "client-secret";
    const { clearOAuthCache } = await import("./client");
    clearOAuthCache();
  });

  it("uses the incoming URL so RFC 9207 iss survives into validation", async () => {
    const openid = await import("openid-client");
    const mocked = vi.mocked(openid.authorizationCodeGrant);
    mocked.mockClear();

    const { exchangeCodeForTokens } = await import("./client");
    const incoming = new URL(
      "https://omnia.example/api/auth/callback?code=c-1&state=state-abc&iss=https%3A%2F%2Faccounts.google.com",
    );

    await exchangeCodeForTokens("c-1", pkce, incoming);

    const [, passedArg] = mocked.mock.calls[0];
    const passedUrl = passedArg as URL;
    // passedUrl retains iss — that's what closes #948 for Google.
    expect(passedUrl.searchParams.get("iss")).toBe("https://accounts.google.com");
    // code/state are re-set by the function itself (defensive).
    expect(passedUrl.searchParams.get("code")).toBe("c-1");
    expect(passedUrl.searchParams.get("state")).toBe("state-abc");
  });

  it("falls back to the configured callback URL when incomingUrl is omitted", async () => {
    // Keeps older test call sites that pass (code, pkce) working
    // without dragging a URL object through every mock.
    const openid = await import("openid-client");
    const mocked = vi.mocked(openid.authorizationCodeGrant);
    mocked.mockClear();

    const { exchangeCodeForTokens } = await import("./client");
    await exchangeCodeForTokens("c-2", pkce);

    const [, passedArg] = mocked.mock.calls[0];
    const passedUrl = passedArg as URL;
    expect(passedUrl.origin + passedUrl.pathname).toBe(
      "https://omnia.example/api/auth/callback",
    );
    expect(passedUrl.searchParams.get("code")).toBe("c-2");
    expect(passedUrl.searchParams.get("state")).toBe("state-abc");
    // No iss on the synthesised URL — that's the bug the fallback path
    // admits; real callers (the callback route) always pass incomingUrl.
    expect(passedUrl.searchParams.has("iss")).toBe(false);
  });
});

describe("client.ts surface (kept lean)", () => {
  beforeEach(async () => {
    process.env.OMNIA_AUTH_MODE = "oauth";
    process.env.OMNIA_BASE_URL = "https://omnia.example";
    process.env.OMNIA_OAUTH_PROVIDER = "google";
    process.env.OMNIA_OAUTH_ISSUER_URL = "https://accounts.google.com";
    process.env.OMNIA_OAUTH_CLIENT_ID = "client-id";
    process.env.OMNIA_OAUTH_CLIENT_SECRET = "client-secret";
    const { clearOAuthCache } = await import("./client");
    clearOAuthCache();
  });

  it("generatePKCE emits a verifier + challenge + state and propagates returnTo", async () => {
    const { generatePKCE } = await import("./client");
    const pkce = await generatePKCE("/return");
    expect(pkce.codeVerifier).toBeTruthy();
    expect(pkce.codeChallenge).toBeTruthy();
    expect(pkce.state).toBeTruthy();
    expect(pkce.returnTo).toBe("/return");
  });

  it("getCallbackUrl uses OMNIA_BASE_URL", async () => {
    const { getCallbackUrl } = await import("./client");
    expect(getCallbackUrl()).toBe("https://omnia.example/api/auth/callback");
  });

  it("buildAuthorizationUrl forwards redirect_uri + scope + state + PKCE", async () => {
    const openid = await import("openid-client");
    const spy = vi.spyOn(openid, "buildAuthorizationUrl");
    spy.mockReturnValue(new URL("https://idp.example/auth?stub=1"));

    const { buildAuthorizationUrl } = await import("./client");
    const href = await buildAuthorizationUrl({
      codeVerifier: "v",
      codeChallenge: "c",
      state: "s",
    });
    expect(href).toBe("https://idp.example/auth?stub=1");
    const [, paramsArg] = spy.mock.calls[0];
    const params = paramsArg as Record<string, string>;
    expect(params.state).toBe("s");
    expect(params.code_challenge).toBe("c");
    expect(params.code_challenge_method).toBe("S256");
    spy.mockRestore();
  });

  it("refreshAccessToken delegates to openid-client refreshTokenGrant", async () => {
    const openid = await import("openid-client");
    const spy = vi.spyOn(openid, "refreshTokenGrant").mockResolvedValue({
      access_token: "at2",
    } as never);

    const { refreshAccessToken } = await import("./client");
    const tokens = await refreshAccessToken("refresh-xyz");
    expect(tokens.access_token).toBe("at2");
    const [, refreshArg] = spy.mock.calls[0];
    expect(refreshArg).toBe("refresh-xyz");
    spy.mockRestore();
  });

  it("getUserInfo delegates to openid-client fetchUserInfo", async () => {
    const openid = await import("openid-client");
    const spy = vi.spyOn(openid, "fetchUserInfo").mockResolvedValue({
      sub: "u1",
    } as never);

    const { getUserInfo } = await import("./client");
    const info = await getUserInfo("access", "u1");
    expect(info.sub).toBe("u1");
    const [, at, sub] = spy.mock.calls[0];
    expect(at).toBe("access");
    expect(sub).toBe("u1");
    spy.mockRestore();
  });

  it("buildEndSessionUrl returns null when the provider metadata omits end_session_endpoint", async () => {
    // The mocked discovery returns empty metadata, so buildEndSessionUrl
    // should return null rather than throw.
    const { buildEndSessionUrl } = await import("./client");
    await expect(buildEndSessionUrl("id-token")).resolves.toBeNull();
  });

  it("buildEndSessionUrl returns a URL when metadata advertises end_session_endpoint", async () => {
    // Override the discovery mock for this case so the metadata
    // exposes end_session_endpoint.
    const openid = await import("openid-client");
    const discoverySpy = vi.spyOn(openid, "discovery").mockResolvedValue({
      serverMetadata: () => ({
        end_session_endpoint: "https://idp.example/logout",
      }),
    } as never);
    const endSessionSpy = vi
      .spyOn(openid, "buildEndSessionUrl")
      .mockReturnValue(new URL("https://idp.example/logout?post_logout_redirect_uri=x"));

    const { buildEndSessionUrl, clearOAuthCache } = await import("./client");
    clearOAuthCache();
    const url = await buildEndSessionUrl("id-hint");
    expect(url).toContain("https://idp.example/logout");

    discoverySpy.mockRestore();
    endSessionSpy.mockRestore();
  });
});

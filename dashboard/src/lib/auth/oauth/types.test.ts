import { describe, it, expect } from "vitest";
import { DEFAULT_CLAIM_MAPPING, DEFAULT_SCOPES, type OAuthTokens } from "./types";

// This is a pure-type file, so these tests exist only to document the
// contract + wake up coverage tooling. If someone adds the accessToken
// field back to OAuthTokens, the first test breaks — which is exactly
// the intent. See the OAuthTokens jsdoc for why it must stay out.
describe("OAuthTokens shape", () => {
  it("does not accept an accessToken field", () => {
    const tokens: OAuthTokens = {
      refreshToken: "rt",
      idToken: "it",
      expiresAt: 123,
      provider: "azure",
    };
    // @ts-expect-error — accessToken intentionally removed; see jsdoc
    tokens.accessToken = "at";
    expect(tokens.refreshToken).toBe("rt");
  });

  it("keeps idToken for RP-initiated logout (id_token_hint)", () => {
    const tokens: OAuthTokens = { provider: "azure", idToken: "it" };
    expect(tokens.idToken).toBe("it");
  });

  it("allows the minimal shape (provider only)", () => {
    const tokens: OAuthTokens = { provider: "google" };
    expect(tokens.provider).toBe("google");
  });
});

describe("OAuth defaults", () => {
  it("exposes OIDC-standard default claim mapping", () => {
    expect(DEFAULT_CLAIM_MAPPING).toEqual({
      username: "preferred_username",
      email: "email",
      displayName: "name",
      groups: "groups",
    });
  });

  it("requests the OIDC baseline scopes", () => {
    expect(DEFAULT_SCOPES).toEqual(["openid", "profile", "email"]);
  });
});

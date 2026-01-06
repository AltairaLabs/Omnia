/**
 * GitHub OAuth provider configuration.
 *
 * Note: GitHub is NOT fully OIDC-compliant. It uses OAuth 2.0 with
 * custom endpoints. Groups are derived from organization membership.
 */

import type { OAuthProviderConfig } from "../types";

export function getGitHubProviderConfig(): Partial<OAuthProviderConfig> {
  return {
    id: "github",
    name: "GitHub",
    // GitHub doesn't support OIDC discovery - must specify endpoints
    supportsDiscovery: false,
    authorizationEndpoint: "https://github.com/login/oauth/authorize",
    tokenEndpoint: "https://github.com/login/oauth/access_token",
    userinfoEndpoint: "https://api.github.com/user",
    scopes: ["read:user", "user:email", "read:org"],
    claims: {
      username: "login",
      email: "email",
      displayName: "name",
      // Organizations are fetched via separate API call
      groups: "organizations",
    },
  };
}

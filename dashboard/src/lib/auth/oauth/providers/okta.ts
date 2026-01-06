/**
 * Okta OAuth provider configuration.
 *
 * Requires OMNIA_OAUTH_OKTA_DOMAIN to be set (e.g., "dev-123456.okta.com").
 */

import type { OAuthProviderConfig } from "../types";

export function getOktaProviderConfig(): Partial<OAuthProviderConfig> {
  const domain = process.env.OMNIA_OAUTH_OKTA_DOMAIN;

  return {
    id: "okta",
    name: "Okta",
    // Okta issuer URL is typically https://{domain} or https://{domain}/oauth2/default
    issuerUrl: domain ? `https://${domain}` : undefined,
    scopes: ["openid", "profile", "email", "groups"],
    claims: {
      username: "preferred_username",
      email: "email",
      displayName: "name",
      groups: "groups",
    },
    supportsDiscovery: true,
  };
}

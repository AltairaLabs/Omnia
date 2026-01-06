/**
 * Google OAuth provider configuration.
 *
 * Supports Google Workspace for group/organization membership.
 * Note: Groups require Google Workspace and admin configuration.
 */

import type { OAuthProviderConfig } from "../types";

export function getGoogleProviderConfig(): Partial<OAuthProviderConfig> {
  return {
    id: "google",
    name: "Google",
    issuerUrl: "https://accounts.google.com",
    scopes: ["openid", "profile", "email"],
    claims: {
      // Google doesn't have preferred_username, use email
      username: "email",
      email: "email",
      displayName: "name",
      // Groups requires Google Workspace admin configuration
      groups: "groups",
    },
    supportsDiscovery: true,
  };
}

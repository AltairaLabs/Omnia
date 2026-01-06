/**
 * Microsoft Azure AD / Entra ID OAuth provider configuration.
 *
 * Supports single-tenant and multi-tenant configurations.
 * Groups require "groups" claim to be enabled in the app registration.
 */

import type { OAuthProviderConfig } from "../types";

export function getAzureProviderConfig(): Partial<OAuthProviderConfig> {
  // Tenant ID from env, or "common" for multi-tenant
  const tenantId = process.env.OMNIA_OAUTH_AZURE_TENANT_ID || "common";

  return {
    id: "azure",
    name: "Microsoft",
    issuerUrl: `https://login.microsoftonline.com/${tenantId}/v2.0`,
    scopes: ["openid", "profile", "email"],
    claims: {
      username: "preferred_username",
      email: "email",
      displayName: "name",
      // Groups require "groups" claim in token configuration
      groups: "groups",
    },
    supportsDiscovery: true,
  };
}

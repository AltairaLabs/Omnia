/**
 * Generic OIDC provider configuration.
 *
 * Works with any OIDC-compliant identity provider that supports discovery.
 */

import {
  DEFAULT_CLAIM_MAPPING,
  DEFAULT_SCOPES,
  type OAuthProviderConfig,
} from "../types";

export function getGenericProviderConfig(): Partial<OAuthProviderConfig> {
  return {
    id: "generic",
    name: "Identity Provider",
    // issuerUrl must be provided via OMNIA_OAUTH_ISSUER_URL
    scopes: [...DEFAULT_SCOPES, "groups"],
    claims: DEFAULT_CLAIM_MAPPING,
    supportsDiscovery: true,
  };
}

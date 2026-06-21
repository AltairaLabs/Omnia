/**
 * OAuth provider configurations.
 */

import type { OAuthProviderConfig, OAuthProviderType } from "../types";
import { getGenericProviderConfig } from "./generic";
import { getGoogleProviderConfig } from "./google";
import { getAzureProviderConfig } from "./azure";
import { getOktaProviderConfig } from "./okta";

/**
 * Provider configuration registry.
 */
const providers: Record<OAuthProviderType, () => Partial<OAuthProviderConfig>> = {
  generic: getGenericProviderConfig,
  google: getGoogleProviderConfig,
  azure: getAzureProviderConfig,
  okta: getOktaProviderConfig,
};

/**
 * Get provider configuration by type.
 */
export function getProviderConfig(type: OAuthProviderType): Partial<OAuthProviderConfig> {
  const provider = providers[type];
  if (!provider) {
    throw new Error(`Unknown OAuth provider: ${type}`);
  }
  return provider();
}

/**
 * Get display name for a provider.
 */
export function getProviderDisplayName(type: OAuthProviderType): string {
  const config = getProviderConfig(type);
  return config.name || type;
}

/**
 * OAuth/OIDC authentication module.
 *
 * Provides direct OAuth/OIDC integration with identity providers.
 *
 * Supported providers:
 * - generic: Any OIDC-compliant provider
 * - google: Google (accounts.google.com)
 * - github: GitHub (OAuth 2.0, not OIDC)
 * - azure: Microsoft Azure AD / Entra ID
 * - okta: Okta
 *
 * Usage:
 *   import { buildAuthorizationUrl, exchangeCodeForTokens } from "@/lib/auth/oauth";
 */

// Types
export * from "./types";

// Client functions
export {
  getOAuthConfig,
  generatePKCE,
  getCallbackUrl,
  buildAuthorizationUrl,
  exchangeCodeForTokens,
  refreshAccessToken,
  getUserInfo,
  buildEndSessionUrl,
  clearOAuthCache,
} from "./client";

// Claim mapping
export { mapClaimsToUser, extractClaims, validateClaims } from "./claims";

// Provider utilities
export {
  getProviderConfig,
  getProviderDisplayName,
  providerSupportsDiscovery,
} from "./providers";

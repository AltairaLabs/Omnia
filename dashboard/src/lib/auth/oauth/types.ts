/**
 * OAuth/OIDC authentication types.
 */

/**
 * Supported OAuth provider types.
 */
export type OAuthProviderType = "generic" | "google" | "github" | "azure" | "okta";

/**
 * Claim mapping configuration.
 * Maps OIDC claims to user fields.
 */
export interface ClaimMapping {
  /** Claim for username (default: preferred_username or sub) */
  username: string;
  /** Claim for email (default: email) */
  email: string;
  /** Claim for display name (default: name) */
  displayName: string;
  /** Claim for groups (default: groups) */
  groups: string;
}

/**
 * OAuth configuration from environment variables.
 */
export interface OAuthConfig {
  /** Provider type: generic or preset name */
  provider: OAuthProviderType;
  /** Client ID */
  clientId: string;
  /** Client secret */
  clientSecret: string;
  /** OIDC Issuer URL (required for generic, optional for presets) */
  issuerUrl?: string;
  /** Scopes to request */
  scopes: string[];
  /** Claim mapping */
  claims: ClaimMapping;
}

/**
 * Provider configuration with all endpoints.
 */
export interface OAuthProviderConfig {
  /** Provider identifier */
  id: OAuthProviderType;
  /** Display name for UI */
  name: string;
  /** OIDC issuer URL (for discovery) */
  issuerUrl?: string;
  /** Default scopes to request */
  scopes: string[];
  /** Default claim mapping */
  claims: ClaimMapping;
  /** Whether provider supports OIDC discovery */
  supportsDiscovery: boolean;
  /** Override authorization endpoint (for non-OIDC providers like GitHub) */
  authorizationEndpoint?: string;
  /** Override token endpoint */
  tokenEndpoint?: string;
  /** Override userinfo endpoint */
  userinfoEndpoint?: string;
}

/**
 * PKCE code challenge pair.
 */
export interface PKCEData {
  /** Code verifier (stored in session) */
  codeVerifier: string;
  /** Code challenge (sent to IdP) */
  codeChallenge: string;
  /** State parameter for CSRF protection */
  state: string;
  /** Return URL after authentication */
  returnTo?: string;
}

/**
 * OAuth tokens stored in session.
 */
export interface OAuthTokens {
  /** Access token for API calls */
  accessToken: string;
  /** Refresh token for token renewal */
  refreshToken?: string;
  /** ID token containing user claims */
  idToken?: string;
  /** Token expiration timestamp (Unix seconds) */
  expiresAt?: number;
  /** Provider used for authentication */
  provider: OAuthProviderType;
}

/**
 * Default claim mapping for OIDC providers.
 */
export const DEFAULT_CLAIM_MAPPING: ClaimMapping = {
  username: "preferred_username",
  email: "email",
  displayName: "name",
  groups: "groups",
};

/**
 * Default scopes for OIDC providers.
 */
export const DEFAULT_SCOPES = ["openid", "profile", "email"];

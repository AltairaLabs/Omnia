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
 * OAuth tokens persisted in the iron-session cookie.
 *
 * Kept minimal on purpose — iron-session encrypts the whole object
 * into a single cookie, and browsers reject cookies >= 4096 bytes
 * (RFC 6265 §4.1.1). Entra ID's ID token alone routinely exceeds
 * that once the user has any group claims, so anything that isn't
 * strictly required must stay out.
 *
 * What we keep + why:
 *   - refreshToken: needed for the refresh flow.
 *   - idToken:      needed for RP-initiated logout (`id_token_hint`).
 *   - expiresAt:    cheap ~10-byte field that drives refresh scheduling.
 *   - provider:     needed for UI + logout endpoint selection.
 *
 * What we no longer store:
 *   - accessToken:  never read. The user info it unlocks is captured
 *                   at callback time via mapClaimsToUser and lives on
 *                   session.user. If a future flow needs a live access
 *                   token, fetch via refreshToken and hold in memory.
 */
export interface OAuthTokens {
  /** Refresh token for token renewal (+ logout revocation where supported). */
  refreshToken?: string;
  /** ID token; only kept because RP-initiated logout needs id_token_hint. */
  idToken?: string;
  /** Token expiration timestamp (Unix seconds). */
  expiresAt?: number;
  /** Provider used for authentication. */
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

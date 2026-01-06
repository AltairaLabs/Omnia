/**
 * OpenID Client wrapper for OAuth/OIDC authentication.
 *
 * Handles OIDC discovery, client creation, and PKCE generation.
 */

import * as client from "openid-client";
import { getAuthConfig } from "../config";
import { getProviderConfig, providerSupportsDiscovery } from "./providers";
import type { PKCEData } from "./types";

// Cached configuration
let cachedConfig: client.Configuration | null = null;

/**
 * Get or create the OIDC client configuration.
 * Uses OIDC discovery to automatically configure endpoints.
 */
export async function getOAuthConfig(): Promise<client.Configuration> {
  if (cachedConfig) {
    return cachedConfig;
  }

  const authConfig = getAuthConfig();
  const providerConfig = getProviderConfig(authConfig.oauth.provider);

  // Determine issuer URL
  const issuerUrl = authConfig.oauth.issuerUrl || providerConfig.issuerUrl;
  if (!issuerUrl) {
    throw new Error(
      `OAuth issuer URL is required. Set OMNIA_OAUTH_ISSUER_URL or use a provider with a default issuer.`
    );
  }

  // Validate client credentials
  if (!authConfig.oauth.clientId) {
    throw new Error("OAuth client ID is required. Set OMNIA_OAUTH_CLIENT_ID.");
  }

  if (!authConfig.oauth.clientSecret) {
    throw new Error(
      "OAuth client secret is required. Set OMNIA_OAUTH_CLIENT_SECRET or OMNIA_OAUTH_CLIENT_SECRET_FILE."
    );
  }

  // Use discovery for OIDC-compliant providers
  if (providerSupportsDiscovery(authConfig.oauth.provider)) {
    cachedConfig = await client.discovery(
      new URL(issuerUrl),
      authConfig.oauth.clientId,
      authConfig.oauth.clientSecret
    );
  } else {
    // For non-discovery providers like GitHub, we'd need manual configuration
    // For now, require discovery support
    throw new Error(
      `Provider ${authConfig.oauth.provider} does not support OIDC discovery. ` +
        `Please use a discovery-capable provider or configure endpoints manually.`
    );
  }

  return cachedConfig;
}

/**
 * Generate PKCE code challenge and state for authorization.
 */
export async function generatePKCE(returnTo?: string): Promise<PKCEData> {
  const codeVerifier = client.randomPKCECodeVerifier();
  const codeChallenge = await client.calculatePKCECodeChallenge(codeVerifier);
  const state = client.randomState();

  return {
    codeVerifier,
    codeChallenge,
    state,
    returnTo,
  };
}

/**
 * Get the OAuth callback URL.
 */
export function getCallbackUrl(): string {
  const config = getAuthConfig();
  return `${config.baseUrl}/api/auth/callback`;
}

/**
 * Build the authorization URL for initiating OAuth flow.
 */
export async function buildAuthorizationUrl(pkce: PKCEData): Promise<string> {
  const config = getAuthConfig();
  const oauthConfig = await getOAuthConfig();

  const authUrl = client.buildAuthorizationUrl(oauthConfig, {
    redirect_uri: getCallbackUrl(),
    scope: config.oauth.scopes.join(" "),
    state: pkce.state,
    code_challenge: pkce.codeChallenge,
    code_challenge_method: "S256",
  });

  return authUrl.href;
}

/**
 * Exchange authorization code for tokens.
 */
export async function exchangeCodeForTokens(
  code: string,
  pkce: PKCEData
): Promise<client.TokenEndpointResponse & client.TokenEndpointResponseHelpers> {
  const oauthConfig = await getOAuthConfig();

  // Build the callback URL with the code and state
  const callbackUrl = new URL(getCallbackUrl());
  callbackUrl.searchParams.set("code", code);
  callbackUrl.searchParams.set("state", pkce.state);

  const tokens = await client.authorizationCodeGrant(oauthConfig, callbackUrl, {
    pkceCodeVerifier: pkce.codeVerifier,
    expectedState: pkce.state,
  });

  return tokens;
}

/**
 * Refresh access token using refresh token.
 */
export async function refreshAccessToken(
  refreshToken: string
): Promise<client.TokenEndpointResponse & client.TokenEndpointResponseHelpers> {
  const oauthConfig = await getOAuthConfig();
  return client.refreshTokenGrant(oauthConfig, refreshToken);
}

/**
 * Get user info from the UserInfo endpoint.
 */
export async function getUserInfo(
  accessToken: string,
  expectedSubject: string
): Promise<client.UserInfoResponse> {
  const oauthConfig = await getOAuthConfig();
  return client.fetchUserInfo(oauthConfig, accessToken, expectedSubject);
}

/**
 * Build end session URL for logout (if supported).
 */
export async function buildEndSessionUrl(idToken?: string): Promise<string | null> {
  try {
    const oauthConfig = await getOAuthConfig();
    const config = getAuthConfig();

    // Check if end_session_endpoint is available
    const metadata = oauthConfig.serverMetadata();
    if (!metadata.end_session_endpoint) {
      return null;
    }

    const params: Record<string, string> = {
      post_logout_redirect_uri: `${config.baseUrl}/login`,
    };

    if (idToken) {
      params.id_token_hint = idToken;
    }

    const endSessionUrl = client.buildEndSessionUrl(oauthConfig, params);
    return endSessionUrl.href;
  } catch {
    return null;
  }
}

/**
 * Clear cached configuration (useful for testing or config changes).
 */
export function clearOAuthCache(): void {
  cachedConfig = null;
}

// Re-export types that callers might need
export type { TokenEndpointResponse, UserInfoResponse } from "openid-client";

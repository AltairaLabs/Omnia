/**
 * OAuth claim-to-user mapping.
 *
 * Maps OIDC ID token claims to the application User type.
 */

import type { TokenEndpointResponse, TokenEndpointResponseHelpers } from "openid-client";
import type { User } from "../types";
import type { AuthConfig, UserRole } from "../config";

/**
 * Claims object from ID token or UserInfo.
 */
type Claims = Record<string, unknown>;

/**
 * Token response with helpers (includes claims() method).
 */
type TokensWithHelpers = TokenEndpointResponse & TokenEndpointResponseHelpers;

/**
 * Map OIDC claims to User object.
 */
export function mapClaimsToUser(
  claims: Claims,
  config: AuthConfig
): User {
  const claimConfig = config.oauth.claims;

  // Extract values from claims using configured mapping
  const sub = claims.sub as string;
  const username = getClaimValue(claims, claimConfig.username) || sub;
  const email = getClaimValue(claims, claimConfig.email);
  const displayName = getClaimValue(claims, claimConfig.displayName);
  const groups = getGroupsClaim(claims, claimConfig.groups);

  // Resolve role from groups using same mapping as proxy auth
  const role = resolveRoleFromGroups(groups, config.roleMapping);

  return {
    id: sub,
    username,
    email,
    displayName,
    groups,
    role,
    provider: "oauth",
  };
}

/**
 * Extract claims from token response.
 * Prefers ID token claims, falls back to empty claims.
 */
export function extractClaims(tokens: TokensWithHelpers): Claims {
  // Use claims() method which decodes the ID token
  const claims = tokens.claims();
  if (claims) {
    return claims as unknown as Claims;
  }

  // Fallback to empty claims (will need UserInfo endpoint)
  return {};
}

/**
 * Get a claim value, supporting nested paths (e.g., "profile.name").
 */
function getClaimValue(claims: Claims, path: string): string | undefined {
  const parts = path.split(".");
  let value: unknown = claims;

  for (const part of parts) {
    if (value && typeof value === "object" && part in value) {
      value = (value as Record<string, unknown>)[part];
    } else {
      return undefined;
    }
  }

  return typeof value === "string" ? value : undefined;
}

/**
 * Extract groups from claims.
 * Handles various formats: string[], string (comma-separated), nested.
 */
function getGroupsClaim(claims: Claims, path: string): string[] {
  const parts = path.split(".");
  let value: unknown = claims;

  for (const part of parts) {
    if (value && typeof value === "object" && part in value) {
      value = (value as Record<string, unknown>)[part];
    } else {
      return [];
    }
  }

  // Handle array of strings
  if (Array.isArray(value)) {
    return value.filter((g): g is string => typeof g === "string");
  }

  // Handle comma-separated string
  if (typeof value === "string") {
    return value
      .split(",")
      .map((g) => g.trim())
      .filter(Boolean);
  }

  return [];
}

/**
 * Resolve role from groups using role mapping.
 * Same logic as proxy mode for consistency.
 */
function resolveRoleFromGroups(
  groups: string[],
  roleMapping: { admin: string[]; editor: string[] }
): UserRole {
  // Check admin first (highest priority)
  for (const adminGroup of roleMapping.admin) {
    if (groups.includes(adminGroup)) {
      return "admin";
    }
  }

  // Check editor
  for (const editorGroup of roleMapping.editor) {
    if (groups.includes(editorGroup)) {
      return "editor";
    }
  }

  // Default to viewer
  return "viewer";
}

/**
 * Check if claims contain required fields.
 */
export function validateClaims(claims: Claims): boolean {
  // Must have sub (subject) claim
  return Boolean(claims.sub && typeof claims.sub === "string");
}

/**
 * OAuth claim-to-user mapping.
 *
 * Maps OIDC ID token claims to the application User type.
 */

import type { TokenEndpointResponse, TokenEndpointResponseHelpers } from "openid-client";
import type { User } from "../types";
import type { AuthConfig, UserRole } from "../config";
import { resolveGroupsOverflow, type GraphTransport } from "./groups-overflow";

/**
 * Claims object from ID token or UserInfo.
 */
type Claims = Record<string, unknown>;

/**
 * Token response with helpers (includes claims() method).
 */
type TokensWithHelpers = TokenEndpointResponse & TokenEndpointResponseHelpers;

/**
 * Map OIDC claims to User object. Synchronous; does NOT resolve
 * Entra `_claim_names.groups` overflow — callers in the OAuth
 * callback / refresh hot path should use mapClaimsToUserAsync to
 * pick up overage groups. This sync entry point stays for callers
 * that genuinely don't have an access token (e.g. tests asserting
 * pure claim-to-user shape).
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
 * mapClaimsToUserAsync is the production entry point. It does
 * everything mapClaimsToUser does, then resolves Entra's
 * `_claim_names.groups` overflow (issue #855) when present. accessToken
 * is the OAuth access token issued alongside the ID token — Microsoft
 * Graph requires it (NOT the ID token) on the Bearer header.
 *
 * Failure modes are absorbed silently (fail-open): if Graph is down /
 * 5xx / 429 / token has no User.Read scope, we surface a console.warn
 * with the operator-actionable reason and resolve the user with
 * `groups: []`. That matches the existing "missing groups → viewer"
 * behaviour for non-overage tokens — overage users get the same
 * degraded experience as a misconfigured tenant rather than being
 * locked out entirely. Operators see the warning in dashboard logs.
 *
 * graphTransport is injectable for tests; production callers omit it
 * to use the global `fetch`.
 */
export async function mapClaimsToUserAsync(
  claims: Claims,
  config: AuthConfig,
  accessToken: string | undefined,
  graphTransport?: GraphTransport,
): Promise<User> {
  const user = mapClaimsToUser(claims, config);

  const result = await resolveGroupsOverflow(
    claims,
    user.groups,
    accessToken,
    graphTransport,
  );

  if (result.kind === "inline") {
    return user;
  }

  if (result.reason) {
    // eslint-disable-next-line no-console
    console.warn(`[oauth] ${result.reason}`);
  }

  // Recompute role with the resolved (or empty-on-failure) group set
  // — admin/editor mapping must use the same list we surface to the
  // user object, otherwise audit logs and downstream RBAC drift.
  const role = resolveRoleFromGroups(result.groups, config.roleMapping);
  return { ...user, groups: result.groups, role };
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

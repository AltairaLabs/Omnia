/**
 * Proxy authentication - extract user from reverse proxy headers.
 *
 * Supports OAuth2 Proxy, Authelia, Keycloak Gatekeeper, Pomerium, etc.
 */

import { headers } from "next/headers";
import { getAuthConfig, type UserRole } from "./config";
import type { User } from "./types";

/**
 * Extract user information from proxy headers.
 * Returns null if no valid user header is present.
 */
export async function getUserFromProxyHeaders(): Promise<User | null> {
  const config = getAuthConfig();
  const headerStore = await headers();

  // Get username from header (required)
  const username = headerStore.get(config.proxy.headerUser);
  if (!username) {
    return null;
  }

  // Get optional headers
  const email = headerStore.get(config.proxy.headerEmail) || undefined;
  const displayName = headerStore.get(config.proxy.headerDisplayName) || undefined;
  const groupsHeader = headerStore.get(config.proxy.headerGroups);
  const groups = groupsHeader
    ? groupsHeader.split(",").map((g) => g.trim()).filter(Boolean)
    : [];

  // Resolve role from groups
  const role = resolveRole(groups, config.roleMapping);

  return {
    id: username,
    username,
    email,
    displayName,
    groups,
    role,
    provider: "proxy",
  };
}

/**
 * Resolve user role from groups using the configured mapping.
 * Falls back to viewer if no groups match.
 */
function resolveRole(
  groups: string[],
  roleMapping: { admin: string[]; editor: string[] }
): UserRole {
  // Check admin first
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
 * Get the headers to forward to Grafana for auth proxy.
 */
export function getGrafanaAuthHeaders(user: User): Record<string, string> {
  return {
    "X-WEBAUTH-USER": user.username,
    ...(user.email && { "X-WEBAUTH-EMAIL": user.email }),
    ...(user.displayName && { "X-WEBAUTH-NAME": user.displayName }),
  };
}

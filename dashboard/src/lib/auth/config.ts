/**
 * Authentication configuration from environment variables.
 *
 * Supports multiple auth modes:
 * - proxy: Header-based auth from reverse proxy (OAuth2 Proxy, Authelia, etc.)
 * - oauth: Direct OAuth/OIDC integration with identity providers
 * - builtin: Local user database with username/password
 * - anonymous: No authentication required
 */

import { readFileSync } from "fs";
import {
  DEFAULT_CLAIM_MAPPING,
  DEFAULT_SCOPES,
  type OAuthConfig,
  type OAuthProviderType,
  type ClaimMapping,
} from "./oauth/types";

// Re-export OAuth types for convenience
export type { OAuthConfig, OAuthProviderType, ClaimMapping };

export type AuthMode = "proxy" | "oauth" | "builtin" | "anonymous";

export type UserRole = "admin" | "editor" | "viewer";

export interface AuthConfig {
  /** Authentication mode */
  mode: AuthMode;
  /** Proxy auth configuration */
  proxy: {
    /** Header containing username */
    headerUser: string;
    /** Header containing email (optional) */
    headerEmail: string;
    /** Header containing groups (optional, comma-separated) */
    headerGroups: string;
    /** Header containing display name (optional) */
    headerDisplayName: string;
    /** Auto-create users on first login */
    autoSignup: boolean;
  };
  /** OAuth/OIDC configuration */
  oauth: OAuthConfig;
  /** Role mapping from groups */
  roleMapping: {
    admin: string[];
    editor: string[];
  };
  /** Session configuration */
  session: {
    /** Session cookie name */
    cookieName: string;
    /** Session secret (32+ chars) */
    secret: string;
    /** Session TTL in seconds */
    ttl: number;
  };
  /** Anonymous access configuration */
  anonymous: {
    /** Role for anonymous users */
    role: UserRole;
  };
  /** Base URL for callbacks */
  baseUrl: string;
}

/**
 * Get auth configuration from environment variables.
 */
export function getAuthConfig(): AuthConfig {
  const mode = (process.env.OMNIA_AUTH_MODE || "anonymous") as AuthMode;

  return {
    mode,
    proxy: {
      headerUser: process.env.OMNIA_AUTH_PROXY_HEADER_USER || "X-Forwarded-User",
      headerEmail: process.env.OMNIA_AUTH_PROXY_HEADER_EMAIL || "X-Forwarded-Email",
      headerGroups: process.env.OMNIA_AUTH_PROXY_HEADER_GROUPS || "X-Forwarded-Groups",
      headerDisplayName: process.env.OMNIA_AUTH_PROXY_HEADER_DISPLAY_NAME || "X-Forwarded-Preferred-Username",
      autoSignup: process.env.OMNIA_AUTH_PROXY_AUTO_SIGNUP !== "false",
    },
    oauth: {
      provider: (process.env.OMNIA_OAUTH_PROVIDER || "generic") as OAuthProviderType,
      clientId: process.env.OMNIA_OAUTH_CLIENT_ID || "",
      clientSecret: getOAuthClientSecret(),
      issuerUrl: process.env.OMNIA_OAUTH_ISSUER_URL,
      scopes: parseList(process.env.OMNIA_OAUTH_SCOPES, DEFAULT_SCOPES),
      claims: {
        username: process.env.OMNIA_OAUTH_CLAIM_USERNAME || DEFAULT_CLAIM_MAPPING.username,
        email: process.env.OMNIA_OAUTH_CLAIM_EMAIL || DEFAULT_CLAIM_MAPPING.email,
        displayName: process.env.OMNIA_OAUTH_CLAIM_DISPLAY_NAME || DEFAULT_CLAIM_MAPPING.displayName,
        groups: process.env.OMNIA_OAUTH_CLAIM_GROUPS || DEFAULT_CLAIM_MAPPING.groups,
      },
    },
    roleMapping: {
      admin: parseGroups(process.env.OMNIA_AUTH_ROLE_ADMIN_GROUPS),
      editor: parseGroups(process.env.OMNIA_AUTH_ROLE_EDITOR_GROUPS),
    },
    session: {
      cookieName: process.env.OMNIA_SESSION_COOKIE_NAME || "omnia_session",
      secret: process.env.OMNIA_SESSION_SECRET || generateDevSecret(),
      ttl: parseInt(process.env.OMNIA_SESSION_TTL || "86400", 10), // 24 hours
    },
    anonymous: {
      role: (process.env.OMNIA_AUTH_ANONYMOUS_ROLE || "viewer") as UserRole,
    },
    baseUrl: process.env.OMNIA_BASE_URL || "http://localhost:3000",
  };
}

/**
 * Parse comma-separated group list.
 */
function parseGroups(value: string | undefined): string[] {
  if (!value) return [];
  return value.split(",").map((g) => g.trim()).filter(Boolean);
}

/**
 * Parse comma-separated list with default.
 */
function parseList(value: string | undefined, defaultValue: string[]): string[] {
  if (!value) return defaultValue;
  return value.split(",").map((s) => s.trim()).filter(Boolean);
}

/**
 * Get OAuth client secret from environment or file.
 * Supports reading from K8s mounted secret file.
 */
function getOAuthClientSecret(): string {
  // Direct environment variable
  if (process.env.OMNIA_OAUTH_CLIENT_SECRET) {
    return process.env.OMNIA_OAUTH_CLIENT_SECRET;
  }

  // File-mounted secret (K8s Secret)
  const secretPath = process.env.OMNIA_OAUTH_CLIENT_SECRET_FILE;
  if (secretPath) {
    try {
      return readFileSync(secretPath, "utf-8").trim();
    } catch {
      console.error(`Failed to read OAuth secret from ${secretPath}`);
    }
  }

  return "";
}

/**
 * Generate a development-only secret.
 * In production, OMNIA_SESSION_SECRET must be set.
 */
function generateDevSecret(): string {
  if (process.env.NODE_ENV === "production") {
    console.warn(
      "WARNING: OMNIA_SESSION_SECRET is not set. Sessions will not persist across restarts."
    );
  }
  // Use a static dev secret so sessions persist during development
  return "omnia-dev-secret-do-not-use-in-production-32";
}

/**
 * Check if authentication is enabled.
 */
export function isAuthEnabled(): boolean {
  return getAuthConfig().mode !== "anonymous";
}

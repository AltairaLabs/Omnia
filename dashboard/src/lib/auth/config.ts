/**
 * Authentication configuration from environment variables.
 *
 * Supports multiple auth modes:
 * - proxy: Header-based auth from reverse proxy (OAuth2 Proxy, Authelia, etc.)
 * - anonymous: No authentication required
 */

export type AuthMode = "proxy" | "anonymous";

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

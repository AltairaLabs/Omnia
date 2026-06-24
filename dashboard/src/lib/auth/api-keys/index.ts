/**
 * API Key authentication module.
 *
 * Provides API key storage and authentication.
 *
 * Store types:
 * - memory: In-memory store (keys lost on restart, good for dev)
 * - file: Read from mounted K8s Secret (GitOps friendly, read-only)
 *
 * Usage:
 *   import { getApiKeyStore, authenticateApiKey } from "@/lib/auth/api-keys";
 *
 *   // Authenticate a request
 *   const user = await authenticateApiKey(request);
 *
 *   // List keys (works in both modes)
 *   const store = getApiKeyStore();
 *   const keys = await store.listByUser(userId);
 *
 *   // Create keys (memory mode only)
 *   const newKey = await store.create(userId, { name: "My Integration" });
 */

export * from "./types";
export { getMemoryApiKeyStore } from "./memory-store";
export { getFileApiKeyStore, FileApiKeyStore } from "./file-store";
export { PostgresApiKeyStore, getPostgresApiKeyStore } from "./postgres-store";

import { headers } from "next/headers";
import type { ApiKeyStore, ApiKey } from "./types";
import { getMemoryApiKeyStore } from "./memory-store";
import { getFileApiKeyStore } from "./file-store";
import { getPostgresApiKeyStore } from "./postgres-store";
import type { User } from "../types";

/**
 * Store type for API keys.
 */
export type ApiKeyStoreType = "memory" | "file" | "postgres";

/**
 * Configuration for API key authentication.
 */
export interface ApiKeyConfig {
  /** Whether API key auth is enabled */
  enabled: boolean;
  /** Store type: memory or file */
  storeType: ApiKeyStoreType;
  /** Path to keys file (when storeType=file) */
  filePath: string;
  /** PostgreSQL connection string (when storeType=postgres) */
  postgresUrl: string;
  /** Maximum keys per user (for memory store) */
  maxKeysPerUser: number;
  /** Default expiration in days (0 = never, for memory store) */
  defaultExpirationDays: number;
  /** Whether key creation is allowed (false for file store) */
  allowCreate: boolean;
}

// Default path for mounted K8s Secret
const DEFAULT_KEYS_FILE_PATH = "/etc/omnia/api-keys/keys.json";

/**
 * Get API key configuration from environment.
 */
export function getApiKeyConfig(): ApiKeyConfig {
  const storeType = (process.env.OMNIA_AUTH_API_KEYS_STORE || "memory") as ApiKeyStoreType;
  const filePath = process.env.OMNIA_AUTH_API_KEYS_FILE_PATH || DEFAULT_KEYS_FILE_PATH;

  return {
    enabled: process.env.OMNIA_AUTH_API_KEYS_ENABLED !== "false",
    storeType,
    filePath,
    postgresUrl:
      process.env.OMNIA_AUTH_API_KEYS_POSTGRES_URL ||
      process.env.OMNIA_BUILTIN_POSTGRES_URL ||
      "",
    maxKeysPerUser: Number.parseInt(
      process.env.OMNIA_AUTH_API_KEYS_MAX_PER_USER || "10",
      10
    ),
    defaultExpirationDays: Number.parseInt(
      process.env.OMNIA_AUTH_API_KEYS_DEFAULT_EXPIRATION || "90",
      10
    ),
    // Only the file store is read-only; memory and postgres support creation
    allowCreate: storeType !== "file",
  };
}

// memoryStoreWarned gates the "memory store while a postgres URL is wired"
// warning so it fires once per process instead of on every store lookup
// (getApiKeyStore runs per request). resetApiKeyAuthWarningsForTest re-arms it.
let memoryStoreWarned = false;

// resetApiKeyAuthWarningsForTest re-arms the once-per-process warnings. Test
// helper only — not used in production code paths.
export function resetApiKeyAuthWarningsForTest(): void {
  memoryStoreWarned = false;
}

/**
 * Get the configured API key store.
 *
 * Store types:
 * - memory: In-memory store (default, for development)
 * - file: File-based store reading from mounted K8s Secret
 * - postgres: PostgreSQL-backed store (production)
 */
export function getApiKeyStore(): ApiKeyStore {
  const config = getApiKeyConfig();

  switch (config.storeType) {
    case "file":
      return getFileApiKeyStore(config.filePath);
    case "postgres":
      return getPostgresApiKeyStore(config.postgresUrl);
    case "memory":
    default:
      // A wired postgres URL + the ephemeral memory store is almost always a
      // misconfig: keys are lost on every dashboard restart, so saved
      // credentials (e.g. deploy-profile tokens) silently stop working. Warn
      // loudly once so it's visible in logs rather than surfacing as opaque
      // 401/403s later (#1582).
      if (config.postgresUrl && !memoryStoreWarned) {
        memoryStoreWarned = true;
        console.warn(
          "[api-keys] OMNIA_AUTH_API_KEYS_POSTGRES_URL is set but the api-key " +
            "store is the ephemeral in-memory store — keys are lost on every " +
            "dashboard restart. Set OMNIA_AUTH_API_KEYS_STORE=postgres to use " +
            "the durable store."
        );
      }
      return getMemoryApiKeyStore();
  }
}

/**
 * Extract API key from request headers.
 *
 * Checks:
 * 1. Authorization: Bearer omnia_sk_...
 * 2. X-API-Key: omnia_sk_...
 */
export async function extractApiKey(): Promise<string | null> {
  const headersList = await headers();

  // Check Authorization header
  const authHeader = headersList.get("authorization");
  if (authHeader?.startsWith("Bearer ")) {
    const token = authHeader.substring(7);
    if (token.startsWith("omnia_sk_")) {
      return token;
    }
  }

  // Check X-API-Key header
  const apiKeyHeader = headersList.get("x-api-key");
  if (apiKeyHeader?.startsWith("omnia_sk_")) {
    return apiKeyHeader;
  }

  return null;
}

/**
 * Authenticate a request using API key.
 *
 * @returns User if authenticated, null if no API key or invalid.
 */
export async function authenticateApiKey(): Promise<User | null> {
  const config = getApiKeyConfig();
  if (!config.enabled) {
    return null;
  }

  const key = await extractApiKey();
  if (!key) {
    return null;
  }

  const store = getApiKeyStore();
  const apiKey = await store.findByKey(key);

  if (!apiKey) {
    return null;
  }

  // Update last used timestamp (fire and forget)
  store.updateLastUsed(apiKey.id).catch(() => {
    // Ignore errors updating last used
  });

  warnIfMissingOwnerSnapshot(apiKey);

  // Create user from API key
  return createUserFromApiKey(apiKey);
}

// warnIfMissingOwnerSnapshot surfaces the silent failure where a
// workspace-scoped key carries no owner identity/group snapshot at all. Such a
// key authenticates but resolves NO workspace role (group roleBindings and
// directGrants both have nothing to match), and a scoped key can't fall back to
// platform-admin (#1561) — so every request 403s with an opaque "Access denied".
// The usual cause is a dashboard image/schema skew: the key was minted before
// the owner-snapshot columns existed (#1568). Legacy unscoped keys are
// unaffected and intentionally not warned. Exported for testing. (#1582)
export function warnIfMissingOwnerSnapshot(apiKey: ApiKey): void {
  const scoped = !!apiKey.workspaces && apiKey.workspaces.length > 0;
  const hasSnapshot =
    !!apiKey.ownerEmail ||
    (!!apiKey.ownerGroups && apiKey.ownerGroups.length > 0);
  if (scoped && !hasSnapshot) {
    console.warn(
      `[api-keys] workspace-scoped key ${apiKey.id} carries no owner snapshot ` +
        "(email/groups) — it will resolve no workspace role and every request " +
        "will 403. Likely minted before the owner-snapshot upgrade (#1568); " +
        "re-issue the key after upgrading the dashboard."
    );
  }
}

/**
 * Build a User from an API key. Carries the owner's snapshot identity/groups
 * (so per-workspace role resolves as if the owner called) and the key's
 * workspace allowlist. Exported for testing.
 */
export function createUserFromApiKey(apiKey: ApiKey): User {
  return {
    id: apiKey.userId,
    username: `apikey:${apiKey.name}`,
    email: apiKey.ownerEmail,
    groups: apiKey.ownerGroups ?? [],
    role: apiKey.role,
    provider: "proxy", // Treat API keys like proxy auth
    apiKeyScope: { workspaces: apiKey.workspaces },
  };
}

/**
 * Check if API key auth is enabled.
 */
export function isApiKeyAuthEnabled(): boolean {
  return getApiKeyConfig().enabled;
}

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

import { headers } from "next/headers";
import type { ApiKeyStore, ApiKey } from "./types";
import { getMemoryApiKeyStore } from "./memory-store";
import { getFileApiKeyStore } from "./file-store";
import type { User } from "../types";

/**
 * Store type for API keys.
 */
export type ApiKeyStoreType = "memory" | "file";

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
    maxKeysPerUser: Number.parseInt(
      process.env.OMNIA_AUTH_API_KEYS_MAX_PER_USER || "10",
      10
    ),
    defaultExpirationDays: Number.parseInt(
      process.env.OMNIA_AUTH_API_KEYS_DEFAULT_EXPIRATION || "90",
      10
    ),
    // File store is read-only
    allowCreate: storeType === "memory",
  };
}

/**
 * Get the configured API key store.
 *
 * Store types:
 * - memory: In-memory store (default, for development)
 * - file: File-based store reading from mounted K8s Secret
 */
export function getApiKeyStore(): ApiKeyStore {
  const config = getApiKeyConfig();

  switch (config.storeType) {
    case "file":
      return getFileApiKeyStore(config.filePath);
    case "memory":
    default:
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

  // Create user from API key
  return createUserFromApiKey(apiKey);
}

/**
 * Create a User object from an API key.
 */
function createUserFromApiKey(apiKey: ApiKey): User {
  return {
    id: apiKey.userId,
    username: `apikey:${apiKey.name}`,
    groups: [],
    role: apiKey.role,
    provider: "proxy", // Treat API keys like proxy auth
  };
}

/**
 * Check if API key auth is enabled.
 */
export function isApiKeyAuthEnabled(): boolean {
  return getApiKeyConfig().enabled;
}

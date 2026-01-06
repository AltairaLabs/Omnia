/**
 * API Key authentication module.
 *
 * Provides API key generation, storage, and authentication.
 *
 * Usage:
 *   import { getApiKeyStore, authenticateApiKey } from "@/lib/auth/api-keys";
 *
 *   // Create a key
 *   const store = getApiKeyStore();
 *   const newKey = await store.create(userId, { name: "My Integration" });
 *
 *   // Authenticate a request
 *   const user = await authenticateApiKey(request);
 */

export * from "./types";
export { getMemoryApiKeyStore } from "./memory-store";

import { headers } from "next/headers";
import type { ApiKeyStore, ApiKey } from "./types";
import { getMemoryApiKeyStore } from "./memory-store";
import { createAnonymousUser, type User } from "../types";
import type { UserRole } from "../config";

/**
 * Get the configured API key store.
 *
 * Currently only supports in-memory storage.
 * Future: Add Redis/PostgreSQL support based on config.
 */
export function getApiKeyStore(): ApiKeyStore {
  // TODO: Check config for store type (memory, redis, postgres)
  return getMemoryApiKeyStore();
}

/**
 * Configuration for API key authentication.
 */
export interface ApiKeyConfig {
  /** Whether API key auth is enabled */
  enabled: boolean;
  /** Maximum keys per user */
  maxKeysPerUser: number;
  /** Default expiration in days (0 = never) */
  defaultExpirationDays: number;
}

/**
 * Get API key configuration from environment.
 */
export function getApiKeyConfig(): ApiKeyConfig {
  return {
    enabled: process.env.OMNIA_AUTH_API_KEYS_ENABLED !== "false",
    maxKeysPerUser: parseInt(
      process.env.OMNIA_AUTH_API_KEYS_MAX_PER_USER || "10",
      10
    ),
    defaultExpirationDays: parseInt(
      process.env.OMNIA_AUTH_API_KEYS_DEFAULT_EXPIRATION || "90",
      10
    ),
  };
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
    role: apiKey.role as UserRole,
    provider: "proxy", // Treat API keys like proxy auth
  };
}

/**
 * Check if API key auth is enabled.
 */
export function isApiKeyAuthEnabled(): boolean {
  return getApiKeyConfig().enabled;
}

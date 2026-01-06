/**
 * API Key types and interfaces.
 */

import type { UserRole } from "../config";

/**
 * API Key prefix for identification.
 */
export const API_KEY_PREFIX = "omnia_sk_";

/**
 * API Key stored in the database.
 */
export interface ApiKey {
  /** Unique identifier */
  id: string;
  /** User who owns this key */
  userId: string;
  /** Display name for the key */
  name: string;
  /** First 8 chars of the key for display (e.g., "omnia_sk_abc12345...") */
  keyPrefix: string;
  /** bcrypt hash of the full key */
  keyHash: string;
  /** Role assigned to this key */
  role: UserRole;
  /** Expiration date (null = never expires) */
  expiresAt: Date | null;
  /** Creation timestamp */
  createdAt: Date;
  /** Last usage timestamp */
  lastUsedAt: Date | null;
}

/**
 * API Key for display (without sensitive data).
 */
export interface ApiKeyInfo {
  id: string;
  userId: string;
  name: string;
  keyPrefix: string;
  role: UserRole;
  expiresAt: Date | null;
  createdAt: Date;
  lastUsedAt: Date | null;
  /** Whether the key is expired */
  isExpired: boolean;
}

/**
 * Newly created API key (includes the full key, shown only once).
 */
export interface NewApiKey extends ApiKeyInfo {
  /** The full API key - only returned once at creation */
  key: string;
}

/**
 * Options for creating a new API key.
 */
export interface CreateApiKeyOptions {
  /** Display name for the key */
  name: string;
  /** Role for the key (defaults to user's role) */
  role?: UserRole;
  /** Expiration in days (null = never) */
  expiresInDays?: number | null;
}

/**
 * Storage interface for API keys.
 * Implementations can use memory, Redis, PostgreSQL, etc.
 */
export interface ApiKeyStore {
  /**
   * Create a new API key.
   * @returns The newly created key with the full key value.
   */
  create(userId: string, options: CreateApiKeyOptions): Promise<NewApiKey>;

  /**
   * List all keys for a user.
   */
  listByUser(userId: string): Promise<ApiKeyInfo[]>;

  /**
   * Find a key by its full value.
   * Returns null if not found or expired.
   */
  findByKey(key: string): Promise<ApiKey | null>;

  /**
   * Delete a key by ID.
   * @returns true if deleted, false if not found.
   */
  delete(keyId: string, userId: string): Promise<boolean>;

  /**
   * Update the lastUsedAt timestamp.
   */
  updateLastUsed(keyId: string): Promise<void>;

  /**
   * Delete all expired keys (cleanup).
   */
  deleteExpired(): Promise<number>;
}

/**
 * Convert an ApiKey to ApiKeyInfo (strips sensitive data).
 */
export function toApiKeyInfo(key: ApiKey): ApiKeyInfo {
  return {
    id: key.id,
    userId: key.userId,
    name: key.name,
    keyPrefix: key.keyPrefix,
    role: key.role,
    expiresAt: key.expiresAt,
    createdAt: key.createdAt,
    lastUsedAt: key.lastUsedAt,
    isExpired: key.expiresAt !== null && key.expiresAt < new Date(),
  };
}

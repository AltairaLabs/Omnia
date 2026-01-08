/**
 * In-memory API key store.
 *
 * This implementation stores keys in memory and is suitable for:
 * - Development and testing
 * - Single-instance deployments where persistence isn't critical
 *
 * For production, use a persistent store like Redis or PostgreSQL.
 */

import { randomBytes } from "node:crypto";
import bcrypt from "bcryptjs";
import {
  API_KEY_PREFIX,
  toApiKeyInfo,
  type ApiKey,
  type ApiKeyInfo,
  type ApiKeyStore,
  type CreateApiKeyOptions,
  type NewApiKey,
} from "./types";

const BCRYPT_ROUNDS = 10;
const KEY_LENGTH = 32; // 256 bits

/**
 * Generate a secure random API key.
 */
function generateKey(): string {
  const randomPart = randomBytes(KEY_LENGTH).toString("base64url");
  return `${API_KEY_PREFIX}${randomPart}`;
}

/**
 * Generate a unique ID.
 */
function generateId(): string {
  return randomBytes(16).toString("hex");
}

/**
 * In-memory API key store.
 */
export class MemoryApiKeyStore implements ApiKeyStore {
  private keys: Map<string, ApiKey> = new Map();

  async create(
    userId: string,
    options: CreateApiKeyOptions
  ): Promise<NewApiKey> {
    const key = generateKey();
    const keyHash = await bcrypt.hash(key, BCRYPT_ROUNDS);
    const keyPrefix = key.substring(0, API_KEY_PREFIX.length + 8) + "...";

    const now = new Date();
    const expiresAt = options.expiresInDays
      ? new Date(now.getTime() + options.expiresInDays * 24 * 60 * 60 * 1000)
      : null;

    const apiKey: ApiKey = {
      id: generateId(),
      userId,
      name: options.name,
      keyPrefix,
      keyHash,
      role: options.role || "viewer",
      expiresAt,
      createdAt: now,
      lastUsedAt: null,
    };

    this.keys.set(apiKey.id, apiKey);

    return {
      ...toApiKeyInfo(apiKey),
      key,
    };
  }

  async listByUser(userId: string): Promise<ApiKeyInfo[]> {
    const userKeys: ApiKeyInfo[] = [];

    for (const key of this.keys.values()) {
      if (key.userId === userId) {
        userKeys.push(toApiKeyInfo(key));
      }
    }

    // Sort by creation date, newest first
    userKeys.sort(
      (a, b) => b.createdAt.getTime() - a.createdAt.getTime()
    );

    return userKeys;
  }

  async findByKey(key: string): Promise<ApiKey | null> {
    // Must start with the correct prefix
    if (!key.startsWith(API_KEY_PREFIX)) {
      return null;
    }

    // Check each stored key
    for (const storedKey of this.keys.values()) {
      // Skip expired keys
      if (storedKey.expiresAt && storedKey.expiresAt < new Date()) {
        continue;
      }

      // Compare with bcrypt
      const matches = await bcrypt.compare(key, storedKey.keyHash);
      if (matches) {
        return storedKey;
      }
    }

    return null;
  }

  async delete(keyId: string, userId: string): Promise<boolean> {
    const key = this.keys.get(keyId);

    // Only delete if the key belongs to the user
    if (key && key.userId === userId) {
      this.keys.delete(keyId);
      return true;
    }

    return false;
  }

  async updateLastUsed(keyId: string): Promise<void> {
    const key = this.keys.get(keyId);
    if (key) {
      key.lastUsedAt = new Date();
    }
  }

  async deleteExpired(): Promise<number> {
    const now = new Date();
    let count = 0;

    for (const [id, key] of this.keys.entries()) {
      if (key.expiresAt && key.expiresAt < now) {
        this.keys.delete(id);
        count++;
      }
    }

    return count;
  }

  /**
   * Clear all keys (for testing).
   */
  clear(): void {
    this.keys.clear();
  }

  /**
   * Get the count of stored keys (for testing).
   */
  get size(): number {
    return this.keys.size;
  }
}

// Singleton instance
let store: MemoryApiKeyStore | null = null;

/**
 * Get the singleton memory store instance.
 */
export function getMemoryApiKeyStore(): MemoryApiKeyStore {
  if (!store) {
    store = new MemoryApiKeyStore();
    console.warn(
      "Using in-memory API key store. Keys will be lost on restart."
    );
  }
  return store;
}

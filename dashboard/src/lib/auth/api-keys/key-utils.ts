/**
 * Shared utilities for API key generation.
 * Used by both the memory store and the Postgres store.
 */

import { randomBytes } from "node:crypto";
import { API_KEY_PREFIX, type CreateApiKeyOptions } from "./types";

/** bcrypt cost factor used when hashing API keys. */
export const BCRYPT_ROUNDS = 10;
const KEY_LENGTH = 32; // 256 bits

/**
 * Generate a secure random API key.
 */
export function generateKey(): string {
  const randomPart = randomBytes(KEY_LENGTH).toString("base64url");
  return `${API_KEY_PREFIX}${randomPart}`;
}

/**
 * Generate a unique ID.
 */
export function generateId(): string {
  return randomBytes(16).toString("hex");
}

/**
 * Compute the display prefix for an API key (first N chars + "...").
 */
export function keyPrefixOf(key: string): string {
  return key.substring(0, API_KEY_PREFIX.length + 8) + "...";
}

/**
 * Compute a key's expiry. expiresInSeconds (when > 0) takes precedence over
 * expiresInDays; null means "never expires".
 */
export function computeExpiresAt(now: Date, options: CreateApiKeyOptions): Date | null {
  if (options.expiresInSeconds && options.expiresInSeconds > 0) {
    return new Date(now.getTime() + options.expiresInSeconds * 1000);
  }
  if (options.expiresInDays) {
    return new Date(now.getTime() + options.expiresInDays * 24 * 60 * 60 * 1000);
  }
  return null;
}

/**
 * Shared utilities for API key generation and hashing.
 * Used by both the memory store and the Postgres store.
 */

import { randomBytes } from "node:crypto";
import bcrypt from "bcryptjs";
import { API_KEY_PREFIX } from "./types";

const BCRYPT_ROUNDS = 10;
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
 * Hash an API key with bcrypt.
 */
export async function hashKey(key: string): Promise<string> {
  return bcrypt.hash(key, BCRYPT_ROUNDS);
}

/**
 * Compute the display prefix for an API key (first N chars + "...").
 */
export function keyPrefix(key: string): string {
  return key.substring(0, API_KEY_PREFIX.length + 8) + "...";
}

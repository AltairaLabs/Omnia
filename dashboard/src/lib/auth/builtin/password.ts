/**
 * Password utilities for built-in authentication.
 *
 * Handles password hashing, verification, and token generation.
 */

import bcrypt from "bcrypt";
import { randomBytes, createHash } from "node:crypto";

/**
 * Number of bcrypt salt rounds.
 * 12 is a good balance of security and performance.
 */
const SALT_ROUNDS = 12;

/**
 * Hash a password using bcrypt.
 */
export async function hashPassword(password: string): Promise<string> {
  return bcrypt.hash(password, SALT_ROUNDS);
}

/**
 * Verify a password against a hash.
 */
export async function verifyPassword(
  password: string,
  hash: string
): Promise<boolean> {
  return bcrypt.compare(password, hash);
}

/**
 * Generate a secure random token.
 * Returns both the plain token (to send to user) and the hash (to store).
 */
export function generateSecureToken(): { token: string; hash: string } {
  const token = randomBytes(32).toString("base64url");
  const hash = createHash("sha256").update(token).digest("hex");
  return { token, hash };
}

/**
 * Hash a token for storage/lookup.
 */
export function hashToken(token: string): string {
  return createHash("sha256").update(token).digest("hex");
}

/**
 * Generate a random user ID.
 */
export function generateUserId(): string {
  return `user_${randomBytes(12).toString("base64url")}`;
}

/**
 * Validate password strength.
 * Returns an array of validation errors (empty if valid).
 */
export function validatePassword(
  password: string,
  minLength: number = 8
): string[] {
  const errors: string[] = [];

  if (password.length < minLength) {
    errors.push(`Password must be at least ${minLength} characters`);
  }

  // Optional: Add more requirements
  // if (!/[A-Z]/.test(password)) {
  //   errors.push("Password must contain at least one uppercase letter");
  // }
  // if (!/[a-z]/.test(password)) {
  //   errors.push("Password must contain at least one lowercase letter");
  // }
  // if (!/[0-9]/.test(password)) {
  //   errors.push("Password must contain at least one number");
  // }

  return errors;
}

/**
 * Validate email format.
 * Uses a simple but efficient regex pattern.
 */
export function validateEmail(email: string): boolean {
  // Simple email validation - check for @ and domain with dot
  // Avoids complex regex that could be slow with malicious input
  if (email.length > 254) return false; // RFC 5321 limit
  const atIndex = email.indexOf("@");
  if (atIndex < 1) return false;
  const domain = email.slice(atIndex + 1);
  return domain.includes(".") && !domain.startsWith(".") && !domain.endsWith(".");
}

/**
 * Validate username format.
 * Allows alphanumeric, underscores, and hyphens, 3-50 characters.
 */
export function validateUsername(username: string): string[] {
  const errors: string[] = [];

  if (username.length < 3) {
    errors.push("Username must be at least 3 characters");
  }
  if (username.length > 50) {
    errors.push("Username must be at most 50 characters");
  }
  if (!/^[a-zA-Z0-9_-]+$/.test(username)) {
    errors.push(
      "Username can only contain letters, numbers, underscores, and hyphens"
    );
  }

  return errors;
}

/**
 * Calculate password strength score (0-4).
 * 0 = very weak, 4 = very strong
 */
export function calculatePasswordStrength(password: string): number {
  let score = 0;

  if (password.length >= 8) score++;
  if (password.length >= 12) score++;
  if (/[a-z]/.test(password) && /[A-Z]/.test(password)) score++;
  if (/\d/.test(password)) score++;
  if (/[^a-zA-Z0-9]/.test(password)) score++;

  return Math.min(score, 4);
}

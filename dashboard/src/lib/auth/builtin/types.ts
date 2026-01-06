/**
 * Built-in authentication types.
 *
 * Types for local user database authentication.
 */

import type { UserRole } from "../config";

/**
 * User record stored in the database.
 */
export interface StoredUser {
  id: string;
  email: string;
  username: string;
  passwordHash: string;
  displayName?: string;
  role: UserRole;
  emailVerified: boolean;
  createdAt: Date;
  updatedAt: Date;
  lastLoginAt?: Date;
  failedLoginAttempts: number;
  lockedUntil?: Date;
}

/**
 * User data for creation (without auto-generated fields).
 */
export interface CreateUserData {
  email: string;
  username: string;
  password: string;
  displayName?: string;
  role?: UserRole;
  emailVerified?: boolean;
}

/**
 * User data for updates.
 */
export interface UpdateUserData {
  email?: string;
  username?: string;
  displayName?: string;
  role?: UserRole;
  emailVerified?: boolean;
}

/**
 * Password reset token record.
 */
export interface PasswordResetToken {
  id: string;
  userId: string;
  tokenHash: string;
  expiresAt: Date;
  usedAt?: Date;
  createdAt: Date;
}

/**
 * Email verification token record.
 */
export interface EmailVerificationToken {
  id: string;
  userId: string;
  tokenHash: string;
  expiresAt: Date;
  usedAt?: Date;
  createdAt: Date;
}

/**
 * User store interface.
 * Implemented by SQLite and PostgreSQL backends.
 */
export interface UserStore {
  /**
   * Initialize the store (create tables, etc.).
   */
  initialize(): Promise<void>;

  /**
   * Close the store connection.
   */
  close(): Promise<void>;

  // User CRUD operations

  /**
   * Create a new user.
   */
  createUser(data: CreateUserData): Promise<StoredUser>;

  /**
   * Get user by ID.
   */
  getUserById(id: string): Promise<StoredUser | null>;

  /**
   * Get user by email.
   */
  getUserByEmail(email: string): Promise<StoredUser | null>;

  /**
   * Get user by username.
   */
  getUserByUsername(username: string): Promise<StoredUser | null>;

  /**
   * Update user.
   */
  updateUser(id: string, data: UpdateUserData): Promise<StoredUser | null>;

  /**
   * Update user's password.
   */
  updatePassword(id: string, passwordHash: string): Promise<void>;

  /**
   * Delete user.
   */
  deleteUser(id: string): Promise<boolean>;

  /**
   * List all users (for admin).
   */
  listUsers(options?: {
    limit?: number;
    offset?: number;
    search?: string;
  }): Promise<{ users: StoredUser[]; total: number }>;

  /**
   * Count total users.
   */
  countUsers(): Promise<number>;

  // Login tracking

  /**
   * Record successful login.
   */
  recordLogin(id: string): Promise<void>;

  /**
   * Record failed login attempt.
   */
  recordFailedLogin(id: string): Promise<void>;

  /**
   * Reset failed login attempts.
   */
  resetFailedLogins(id: string): Promise<void>;

  /**
   * Lock user account until specified time.
   */
  lockUser(id: string, until: Date): Promise<void>;

  /**
   * Unlock user account.
   */
  unlockUser(id: string): Promise<void>;

  // Password reset tokens

  /**
   * Create password reset token.
   */
  createPasswordResetToken(
    userId: string,
    tokenHash: string,
    expiresAt: Date
  ): Promise<PasswordResetToken>;

  /**
   * Get password reset token by hash.
   */
  getPasswordResetToken(tokenHash: string): Promise<PasswordResetToken | null>;

  /**
   * Mark password reset token as used.
   */
  markPasswordResetTokenUsed(id: string): Promise<void>;

  /**
   * Delete expired password reset tokens.
   */
  deleteExpiredPasswordResetTokens(): Promise<number>;

  // Email verification tokens

  /**
   * Create email verification token.
   */
  createEmailVerificationToken(
    userId: string,
    tokenHash: string,
    expiresAt: Date
  ): Promise<EmailVerificationToken>;

  /**
   * Get email verification token by hash.
   */
  getEmailVerificationToken(
    tokenHash: string
  ): Promise<EmailVerificationToken | null>;

  /**
   * Mark email verification token as used and verify user's email.
   */
  verifyEmail(tokenId: string, userId: string): Promise<void>;

  /**
   * Delete expired email verification tokens.
   */
  deleteExpiredEmailVerificationTokens(): Promise<number>;
}

/**
 * User store type configuration.
 */
export type UserStoreType = "sqlite" | "postgresql";

/**
 * Built-in auth configuration.
 */
export interface BuiltinAuthConfig {
  /** User store type */
  storeType: UserStoreType;

  /** SQLite database path (for sqlite store) */
  sqlitePath?: string;

  /** PostgreSQL connection URL (for postgresql store) */
  postgresUrl?: string;

  /** Allow new user signups */
  allowSignup: boolean;

  /** Require email verification */
  verifyEmail: boolean;

  /** Minimum password length */
  minPasswordLength: number;

  /** Maximum failed login attempts before lockout */
  maxFailedAttempts: number;

  /** Lockout duration in seconds */
  lockoutDuration: number;

  /** Password reset token expiration in seconds */
  resetTokenExpiration: number;

  /** Email verification token expiration in seconds */
  verificationTokenExpiration: number;

  /** Default admin username (created on first run) */
  adminUsername?: string;

  /** Default admin email (created on first run) */
  adminEmail?: string;

  /** Default admin password (created on first run, should be changed) */
  adminPassword?: string;
}

/**
 * Default configuration values.
 */
export const DEFAULT_BUILTIN_CONFIG: Omit<
  BuiltinAuthConfig,
  "storeType" | "sqlitePath" | "postgresUrl"
> = {
  allowSignup: false,
  verifyEmail: false,
  minPasswordLength: 8,
  maxFailedAttempts: 5,
  lockoutDuration: 900, // 15 minutes
  resetTokenExpiration: 3600, // 1 hour
  verificationTokenExpiration: 86400, // 24 hours
};

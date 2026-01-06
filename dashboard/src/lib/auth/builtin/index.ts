/**
 * Built-in authentication module.
 *
 * Provides local user database authentication with pluggable storage.
 */

import type { UserStore, BuiltinAuthConfig, UserStoreType } from "./types";
import { SQLiteUserStore } from "./sqlite-store";
import { PostgresUserStore } from "./postgres-store";
import { DEFAULT_BUILTIN_CONFIG } from "./types";
import path from "path";

// Singleton store instance
let store: UserStore | null = null;
let storeConfig: BuiltinAuthConfig | null = null;

/**
 * Get the builtin auth configuration from environment variables.
 */
export function getBuiltinConfig(): BuiltinAuthConfig {
  const storeType = (process.env.OMNIA_BUILTIN_STORE_TYPE ||
    "sqlite") as UserStoreType;

  return {
    storeType,
    sqlitePath:
      process.env.OMNIA_BUILTIN_SQLITE_PATH ||
      path.join(process.cwd(), "data", "omnia-users.db"),
    postgresUrl: process.env.OMNIA_BUILTIN_POSTGRES_URL,

    allowSignup: process.env.OMNIA_BUILTIN_ALLOW_SIGNUP === "true",
    verifyEmail: process.env.OMNIA_BUILTIN_VERIFY_EMAIL === "true",
    minPasswordLength: parseInt(
      process.env.OMNIA_BUILTIN_MIN_PASSWORD_LENGTH ||
        String(DEFAULT_BUILTIN_CONFIG.minPasswordLength),
      10
    ),
    maxFailedAttempts: parseInt(
      process.env.OMNIA_BUILTIN_MAX_FAILED_ATTEMPTS ||
        String(DEFAULT_BUILTIN_CONFIG.maxFailedAttempts),
      10
    ),
    lockoutDuration: parseInt(
      process.env.OMNIA_BUILTIN_LOCKOUT_DURATION ||
        String(DEFAULT_BUILTIN_CONFIG.lockoutDuration),
      10
    ),
    resetTokenExpiration: parseInt(
      process.env.OMNIA_BUILTIN_RESET_TOKEN_EXPIRATION ||
        String(DEFAULT_BUILTIN_CONFIG.resetTokenExpiration),
      10
    ),
    verificationTokenExpiration: parseInt(
      process.env.OMNIA_BUILTIN_VERIFICATION_TOKEN_EXPIRATION ||
        String(DEFAULT_BUILTIN_CONFIG.verificationTokenExpiration),
      10
    ),

    adminUsername: process.env.OMNIA_BUILTIN_ADMIN_USERNAME,
    adminEmail: process.env.OMNIA_BUILTIN_ADMIN_EMAIL,
    adminPassword: process.env.OMNIA_BUILTIN_ADMIN_PASSWORD,
  };
}

/**
 * Create a user store based on configuration.
 */
function createStore(config: BuiltinAuthConfig): UserStore {
  switch (config.storeType) {
    case "sqlite":
      if (!config.sqlitePath) {
        throw new Error("SQLite path is required for sqlite store type");
      }
      return new SQLiteUserStore(config.sqlitePath);

    case "postgresql":
      if (!config.postgresUrl) {
        throw new Error(
          "PostgreSQL URL is required for postgresql store type. Set OMNIA_BUILTIN_POSTGRES_URL."
        );
      }
      return new PostgresUserStore(config.postgresUrl);

    default:
      throw new Error(`Unknown store type: ${config.storeType}`);
  }
}

/**
 * Get or create the user store singleton.
 */
export async function getUserStore(): Promise<UserStore> {
  if (store) {
    return store;
  }

  const config = getBuiltinConfig();
  storeConfig = config;

  // Ensure data directory exists for SQLite
  if (config.storeType === "sqlite" && config.sqlitePath) {
    const { mkdirSync } = await import("fs");
    const dir = path.dirname(config.sqlitePath);
    mkdirSync(dir, { recursive: true });
  }

  store = createStore(config);
  await store.initialize();

  // Seed admin user if configured and no users exist
  await seedAdminUser(store, config);

  return store;
}

/**
 * Seed the admin user if configured and no users exist.
 */
async function seedAdminUser(
  store: UserStore,
  config: BuiltinAuthConfig
): Promise<void> {
  if (!config.adminUsername || !config.adminEmail || !config.adminPassword) {
    return;
  }

  const userCount = await store.countUsers();
  if (userCount > 0) {
    return;
  }

  console.log("Seeding initial admin user...");

  await store.createUser({
    username: config.adminUsername,
    email: config.adminEmail,
    password: config.adminPassword,
    role: "admin",
    emailVerified: true,
  });

  console.log(`Admin user '${config.adminUsername}' created successfully.`);
  console.log("IMPORTANT: Change the admin password after first login!");
}

/**
 * Close the user store connection.
 */
export async function closeUserStore(): Promise<void> {
  if (store) {
    await store.close();
    store = null;
    storeConfig = null;
  }
}

/**
 * Get the current builtin auth configuration (if initialized).
 */
export function getCurrentConfig(): BuiltinAuthConfig | null {
  return storeConfig;
}

// Re-export types and utilities
export type {
  UserStore,
  StoredUser,
  CreateUserData,
  UpdateUserData,
  PasswordResetToken,
  EmailVerificationToken,
  BuiltinAuthConfig,
  UserStoreType,
} from "./types";

export {
  hashPassword,
  verifyPassword,
  generateSecureToken,
  hashToken,
  validatePassword,
  validateEmail,
  validateUsername,
  calculatePasswordStrength,
} from "./password";

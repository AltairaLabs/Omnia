/**
 * PostgreSQL implementation of the user store.
 *
 * Uses pg (node-postgres) for PostgreSQL access.
 * Suitable for production multi-instance deployments.
 */

import { Pool } from "pg";
import type {
  UserStore,
  StoredUser,
  CreateUserData,
  UpdateUserData,
  PasswordResetToken,
  EmailVerificationToken,
} from "./types";
import { hashPassword, generateUserId } from "./password";

/**
 * PostgreSQL user store implementation.
 */
export class PostgresUserStore implements UserStore {
  private pool: Pool;

  constructor(connectionString: string) {
    this.pool = new Pool({
      connectionString,
      max: 20,
      idleTimeoutMillis: 30000,
      connectionTimeoutMillis: 2000,
    });
  }

  async initialize(): Promise<void> {
    const client = await this.pool.connect();
    try {
      // Create users table
      await client.query(`
        CREATE TABLE IF NOT EXISTS users (
          id TEXT PRIMARY KEY,
          email TEXT UNIQUE NOT NULL,
          username TEXT UNIQUE NOT NULL,
          password_hash TEXT NOT NULL,
          display_name TEXT,
          role TEXT NOT NULL DEFAULT 'viewer',
          email_verified BOOLEAN NOT NULL DEFAULT FALSE,
          created_at TIMESTAMPTZ NOT NULL,
          updated_at TIMESTAMPTZ NOT NULL,
          last_login_at TIMESTAMPTZ,
          failed_login_attempts INTEGER NOT NULL DEFAULT 0,
          locked_until TIMESTAMPTZ
        )
      `);

      // Create password reset tokens table
      await client.query(`
        CREATE TABLE IF NOT EXISTS password_reset_tokens (
          id TEXT PRIMARY KEY,
          user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
          token_hash TEXT UNIQUE NOT NULL,
          expires_at TIMESTAMPTZ NOT NULL,
          used_at TIMESTAMPTZ,
          created_at TIMESTAMPTZ NOT NULL
        )
      `);

      // Create email verification tokens table
      await client.query(`
        CREATE TABLE IF NOT EXISTS email_verification_tokens (
          id TEXT PRIMARY KEY,
          user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
          token_hash TEXT UNIQUE NOT NULL,
          expires_at TIMESTAMPTZ NOT NULL,
          used_at TIMESTAMPTZ,
          created_at TIMESTAMPTZ NOT NULL
        )
      `);

      // Create indexes
      await client.query(`
        CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
        CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
        CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_hash ON password_reset_tokens(token_hash);
        CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_hash ON email_verification_tokens(token_hash);
      `);
    } finally {
      client.release();
    }
  }

  async close(): Promise<void> {
    await this.pool.end();
  }

  // User CRUD operations

  async createUser(data: CreateUserData): Promise<StoredUser> {
    const id = generateUserId();
    const now = new Date();
    const passwordHash = await hashPassword(data.password);

    const result = await this.pool.query(
      `
      INSERT INTO users (id, email, username, password_hash, display_name, role, email_verified, created_at, updated_at)
      VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
      RETURNING *
      `,
      [
        id,
        data.email.toLowerCase(),
        data.username.toLowerCase(),
        passwordHash,
        data.displayName || null,
        data.role || "viewer",
        data.emailVerified || false,
        now,
        now,
      ]
    );

    return this.rowToUser(result.rows[0]);
  }

  async getUserById(id: string): Promise<StoredUser | null> {
    const result = await this.pool.query(
      "SELECT * FROM users WHERE id = $1",
      [id]
    );
    return result.rows[0] ? this.rowToUser(result.rows[0]) : null;
  }

  async getUserByEmail(email: string): Promise<StoredUser | null> {
    const result = await this.pool.query(
      "SELECT * FROM users WHERE email = $1",
      [email.toLowerCase()]
    );
    return result.rows[0] ? this.rowToUser(result.rows[0]) : null;
  }

  async getUserByUsername(username: string): Promise<StoredUser | null> {
    const result = await this.pool.query(
      "SELECT * FROM users WHERE username = $1",
      [username.toLowerCase()]
    );
    return result.rows[0] ? this.rowToUser(result.rows[0]) : null;
  }

  async updateUser(
    id: string,
    data: UpdateUserData
  ): Promise<StoredUser | null> {
    const updates: string[] = [];
    const values: (string | boolean)[] = [];
    let paramIndex = 1;

    if (data.email !== undefined) {
      updates.push(`email = $${paramIndex++}`);
      values.push(data.email.toLowerCase());
    }
    if (data.username !== undefined) {
      updates.push(`username = $${paramIndex++}`);
      values.push(data.username.toLowerCase());
    }
    if (data.displayName !== undefined) {
      updates.push(`display_name = $${paramIndex++}`);
      values.push(data.displayName);
    }
    if (data.role !== undefined) {
      updates.push(`role = $${paramIndex++}`);
      values.push(data.role);
    }
    if (data.emailVerified !== undefined) {
      updates.push(`email_verified = $${paramIndex++}`);
      values.push(data.emailVerified);
    }

    if (updates.length === 0) {
      return this.getUserById(id);
    }

    updates.push(`updated_at = $${paramIndex++}`);
    values.push(new Date().toISOString());
    values.push(id);

    // NOSONAR - Dynamic query uses parameterized placeholders ($1, $2, etc.) for safety
    const result = await this.pool.query(
      `UPDATE users SET ${updates.join(", ")} WHERE id = $${paramIndex} RETURNING *`,
      values
    );

    return result.rows[0] ? this.rowToUser(result.rows[0]) : null;
  }

  async updatePassword(id: string, passwordHash: string): Promise<void> {
    await this.pool.query(
      "UPDATE users SET password_hash = $1, updated_at = $2 WHERE id = $3",
      [passwordHash, new Date(), id]
    );
  }

  async deleteUser(id: string): Promise<boolean> {
    const result = await this.pool.query(
      "DELETE FROM users WHERE id = $1",
      [id]
    );
    return (result.rowCount ?? 0) > 0;
  }

  async listUsers(options?: {
    limit?: number;
    offset?: number;
    search?: string;
  }): Promise<{ users: StoredUser[]; total: number }> {
    const limit = options?.limit || 50;
    const offset = options?.offset || 0;
    const search = options?.search;

    let query = "SELECT * FROM users";
    let countQuery = "SELECT COUNT(*) as count FROM users";
    const params: (string | number)[] = [];

    if (search) {
      query += " WHERE email ILIKE $1 OR username ILIKE $1 OR display_name ILIKE $1";
      countQuery += " WHERE email ILIKE $1 OR username ILIKE $1 OR display_name ILIKE $1";
      params.push(`%${search}%`);
    }

    query += ` ORDER BY created_at DESC LIMIT $${params.length + 1} OFFSET $${params.length + 2}`;

    const countResult = await this.pool.query(countQuery, params);
    const result = await this.pool.query(query, [...params, limit, offset]);

    return {
      users: result.rows.map((row) => this.rowToUser(row)),
      total: parseInt(countResult.rows[0].count, 10),
    };
  }

  async countUsers(): Promise<number> {
    const result = await this.pool.query("SELECT COUNT(*) as count FROM users");
    return parseInt(result.rows[0].count, 10);
  }

  // Login tracking

  async recordLogin(id: string): Promise<void> {
    await this.pool.query(
      "UPDATE users SET last_login_at = $1, failed_login_attempts = 0, locked_until = NULL WHERE id = $2",
      [new Date(), id]
    );
  }

  async recordFailedLogin(id: string): Promise<void> {
    await this.pool.query(
      "UPDATE users SET failed_login_attempts = failed_login_attempts + 1 WHERE id = $1",
      [id]
    );
  }

  async resetFailedLogins(id: string): Promise<void> {
    await this.pool.query(
      "UPDATE users SET failed_login_attempts = 0 WHERE id = $1",
      [id]
    );
  }

  async lockUser(id: string, until: Date): Promise<void> {
    await this.pool.query(
      "UPDATE users SET locked_until = $1 WHERE id = $2",
      [until, id]
    );
  }

  async unlockUser(id: string): Promise<void> {
    await this.pool.query(
      "UPDATE users SET locked_until = NULL, failed_login_attempts = 0 WHERE id = $1",
      [id]
    );
  }

  // Password reset tokens

  async createPasswordResetToken(
    userId: string,
    tokenHash: string,
    expiresAt: Date
  ): Promise<PasswordResetToken> {
    const id = `prt_${generateUserId()}`;
    const now = new Date();

    const result = await this.pool.query(
      `
      INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, created_at)
      VALUES ($1, $2, $3, $4, $5)
      RETURNING *
      `,
      [id, userId, tokenHash, expiresAt, now]
    );

    return this.rowToPasswordResetToken(result.rows[0]);
  }

  async getPasswordResetToken(
    tokenHash: string
  ): Promise<PasswordResetToken | null> {
    const result = await this.pool.query(
      "SELECT * FROM password_reset_tokens WHERE token_hash = $1",
      [tokenHash]
    );
    return result.rows[0] ? this.rowToPasswordResetToken(result.rows[0]) : null;
  }

  async markPasswordResetTokenUsed(id: string): Promise<void> {
    await this.pool.query(
      "UPDATE password_reset_tokens SET used_at = $1 WHERE id = $2",
      [new Date(), id]
    );
  }

  async deleteExpiredPasswordResetTokens(): Promise<number> {
    const result = await this.pool.query(
      "DELETE FROM password_reset_tokens WHERE expires_at < $1 OR used_at IS NOT NULL",
      [new Date()]
    );
    return result.rowCount ?? 0;
  }

  // Email verification tokens

  async createEmailVerificationToken(
    userId: string,
    tokenHash: string,
    expiresAt: Date
  ): Promise<EmailVerificationToken> {
    const id = `evt_${generateUserId()}`;
    const now = new Date();

    const result = await this.pool.query(
      `
      INSERT INTO email_verification_tokens (id, user_id, token_hash, expires_at, created_at)
      VALUES ($1, $2, $3, $4, $5)
      RETURNING *
      `,
      [id, userId, tokenHash, expiresAt, now]
    );

    return this.rowToEmailVerificationToken(result.rows[0]);
  }

  async getEmailVerificationToken(
    tokenHash: string
  ): Promise<EmailVerificationToken | null> {
    const result = await this.pool.query(
      "SELECT * FROM email_verification_tokens WHERE token_hash = $1",
      [tokenHash]
    );
    return result.rows[0]
      ? this.rowToEmailVerificationToken(result.rows[0])
      : null;
  }

  async verifyEmail(tokenId: string, userId: string): Promise<void> {
    const client = await this.pool.connect();
    try {
      await client.query("BEGIN");

      // Mark token as used
      await client.query(
        "UPDATE email_verification_tokens SET used_at = $1 WHERE id = $2",
        [new Date(), tokenId]
      );

      // Verify user's email
      await client.query(
        "UPDATE users SET email_verified = TRUE, updated_at = $1 WHERE id = $2",
        [new Date(), userId]
      );

      await client.query("COMMIT");
    } catch (error) {
      await client.query("ROLLBACK");
      throw error;
    } finally {
      client.release();
    }
  }

  async deleteExpiredEmailVerificationTokens(): Promise<number> {
    const result = await this.pool.query(
      "DELETE FROM email_verification_tokens WHERE expires_at < $1 OR used_at IS NOT NULL",
      [new Date()]
    );
    return result.rowCount ?? 0;
  }

  // Helper methods

  private rowToUser(row: UserRow): StoredUser {
    return {
      id: row.id,
      email: row.email,
      username: row.username,
      passwordHash: row.password_hash,
      displayName: row.display_name || undefined,
      role: row.role as StoredUser["role"],
      emailVerified: row.email_verified,
      createdAt: new Date(row.created_at),
      updatedAt: new Date(row.updated_at),
      lastLoginAt: row.last_login_at ? new Date(row.last_login_at) : undefined,
      failedLoginAttempts: row.failed_login_attempts,
      lockedUntil: row.locked_until ? new Date(row.locked_until) : undefined,
    };
  }

  private rowToToken<T extends PasswordResetToken | EmailVerificationToken>(
    row: PasswordResetTokenRow | EmailVerificationTokenRow
  ): T {
    return {
      id: row.id,
      userId: row.user_id,
      tokenHash: row.token_hash,
      expiresAt: new Date(row.expires_at),
      usedAt: row.used_at ? new Date(row.used_at) : undefined,
      createdAt: new Date(row.created_at),
    } as T;
  }

  private rowToPasswordResetToken(row: PasswordResetTokenRow): PasswordResetToken {
    return this.rowToToken<PasswordResetToken>(row);
  }

  private rowToEmailVerificationToken(
    row: EmailVerificationTokenRow
  ): EmailVerificationToken {
    return this.rowToToken<EmailVerificationToken>(row);
  }
}

// Row types for PostgreSQL results
interface UserRow {
  id: string;
  email: string;
  username: string;
  password_hash: string;
  display_name: string | null;
  role: string;
  email_verified: boolean;
  created_at: Date;
  updated_at: Date;
  last_login_at: Date | null;
  failed_login_attempts: number;
  locked_until: Date | null;
}

interface PasswordResetTokenRow {
  id: string;
  user_id: string;
  token_hash: string;
  expires_at: Date;
  used_at: Date | null;
  created_at: Date;
}

interface EmailVerificationTokenRow {
  id: string;
  user_id: string;
  token_hash: string;
  expires_at: Date;
  used_at: Date | null;
  created_at: Date;
}

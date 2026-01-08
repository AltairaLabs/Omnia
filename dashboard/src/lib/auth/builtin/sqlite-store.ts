/**
 * SQLite implementation of the user store.
 *
 * Uses better-sqlite3 for synchronous, high-performance SQLite access.
 * Suitable for single-instance deployments and development.
 */

import Database from "better-sqlite3";
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
 * SQLite user store implementation.
 */
export class SQLiteUserStore implements UserStore {
  private db: Database.Database;

  constructor(dbPath: string) {
    this.db = new Database(dbPath);
    this.db.pragma("journal_mode = WAL");
    this.db.pragma("foreign_keys = ON");
  }

  async initialize(): Promise<void> {
    // Create users table
    this.db.exec(`
      CREATE TABLE IF NOT EXISTS users (
        id TEXT PRIMARY KEY,
        email TEXT UNIQUE NOT NULL,
        username TEXT UNIQUE NOT NULL,
        password_hash TEXT NOT NULL,
        display_name TEXT,
        role TEXT NOT NULL DEFAULT 'viewer',
        email_verified INTEGER NOT NULL DEFAULT 0,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL,
        last_login_at TEXT,
        failed_login_attempts INTEGER NOT NULL DEFAULT 0,
        locked_until TEXT
      )
    `);

    // Create password reset tokens table
    this.db.exec(`
      CREATE TABLE IF NOT EXISTS password_reset_tokens (
        id TEXT PRIMARY KEY,
        user_id TEXT NOT NULL,
        token_hash TEXT UNIQUE NOT NULL,
        expires_at TEXT NOT NULL,
        used_at TEXT,
        created_at TEXT NOT NULL,
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
      )
    `);

    // Create email verification tokens table
    this.db.exec(`
      CREATE TABLE IF NOT EXISTS email_verification_tokens (
        id TEXT PRIMARY KEY,
        user_id TEXT NOT NULL,
        token_hash TEXT UNIQUE NOT NULL,
        expires_at TEXT NOT NULL,
        used_at TEXT,
        created_at TEXT NOT NULL,
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
      )
    `);

    // Create indexes
    this.db.exec(`
      CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
      CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
      CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_hash ON password_reset_tokens(token_hash);
      CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_hash ON email_verification_tokens(token_hash);
    `);
  }

  async close(): Promise<void> {
    this.db.close();
  }

  // User CRUD operations

  async createUser(data: CreateUserData): Promise<StoredUser> {
    const id = generateUserId();
    const now = new Date().toISOString();
    const passwordHash = await hashPassword(data.password);

    const stmt = this.db.prepare(`
      INSERT INTO users (id, email, username, password_hash, display_name, role, email_verified, created_at, updated_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `);

    stmt.run(
      id,
      data.email.toLowerCase(),
      data.username.toLowerCase(),
      passwordHash,
      data.displayName || null,
      data.role || "viewer",
      data.emailVerified ? 1 : 0,
      now,
      now
    );

    return this.getUserById(id) as Promise<StoredUser>;
  }

  async getUserById(id: string): Promise<StoredUser | null> {
    const stmt = this.db.prepare("SELECT * FROM users WHERE id = ?");
    const row = stmt.get(id) as UserRow | undefined;
    return row ? this.rowToUser(row) : null;
  }

  async getUserByEmail(email: string): Promise<StoredUser | null> {
    const stmt = this.db.prepare("SELECT * FROM users WHERE email = ?");
    const row = stmt.get(email.toLowerCase()) as UserRow | undefined;
    return row ? this.rowToUser(row) : null;
  }

  async getUserByUsername(username: string): Promise<StoredUser | null> {
    const stmt = this.db.prepare("SELECT * FROM users WHERE username = ?");
    const row = stmt.get(username.toLowerCase()) as UserRow | undefined;
    return row ? this.rowToUser(row) : null;
  }

  async updateUser(
    id: string,
    data: UpdateUserData
  ): Promise<StoredUser | null> {
    const updates: string[] = [];
    const values: (string | number)[] = [];

    if (data.email !== undefined) {
      updates.push("email = ?");
      values.push(data.email.toLowerCase());
    }
    if (data.username !== undefined) {
      updates.push("username = ?");
      values.push(data.username.toLowerCase());
    }
    if (data.displayName !== undefined) {
      updates.push("display_name = ?");
      values.push(data.displayName);
    }
    if (data.role !== undefined) {
      updates.push("role = ?");
      values.push(data.role);
    }
    if (data.emailVerified !== undefined) {
      updates.push("email_verified = ?");
      values.push(data.emailVerified ? 1 : 0);
    }

    if (updates.length === 0) {
      return this.getUserById(id);
    }

    updates.push("updated_at = ?");
    values.push(new Date().toISOString());
    values.push(id);

    const stmt = this.db.prepare(
      `UPDATE users SET ${updates.join(", ")} WHERE id = ?`
    );
    stmt.run(...values);

    return this.getUserById(id);
  }

  async updatePassword(id: string, passwordHash: string): Promise<void> {
    const stmt = this.db.prepare(
      "UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?"
    );
    stmt.run(passwordHash, new Date().toISOString(), id);
  }

  async deleteUser(id: string): Promise<boolean> {
    const stmt = this.db.prepare("DELETE FROM users WHERE id = ?");
    const result = stmt.run(id);
    return result.changes > 0;
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
      const searchClause =
        " WHERE email LIKE ? OR username LIKE ? OR display_name LIKE ?";
      query += searchClause;
      countQuery += searchClause;
      const searchParam = `%${search}%`;
      params.push(searchParam, searchParam, searchParam);
    }

    query += " ORDER BY created_at DESC LIMIT ? OFFSET ?";

    const countStmt = this.db.prepare(countQuery);
    const countRow = countStmt.get(...params) as { count: number };

    const stmt = this.db.prepare(query);
    const rows = stmt.all(...params, limit, offset) as UserRow[];

    return {
      users: rows.map((row) => this.rowToUser(row)),
      total: countRow.count,
    };
  }

  async countUsers(): Promise<number> {
    const stmt = this.db.prepare("SELECT COUNT(*) as count FROM users");
    const row = stmt.get() as { count: number };
    return row.count;
  }

  // Login tracking

  async recordLogin(id: string): Promise<void> {
    const stmt = this.db.prepare(
      "UPDATE users SET last_login_at = ?, failed_login_attempts = 0, locked_until = NULL WHERE id = ?"
    );
    stmt.run(new Date().toISOString(), id);
  }

  async recordFailedLogin(id: string): Promise<void> {
    const stmt = this.db.prepare(
      "UPDATE users SET failed_login_attempts = failed_login_attempts + 1 WHERE id = ?"
    );
    stmt.run(id);
  }

  async resetFailedLogins(id: string): Promise<void> {
    const stmt = this.db.prepare(
      "UPDATE users SET failed_login_attempts = 0 WHERE id = ?"
    );
    stmt.run(id);
  }

  async lockUser(id: string, until: Date): Promise<void> {
    const stmt = this.db.prepare(
      "UPDATE users SET locked_until = ? WHERE id = ?"
    );
    stmt.run(until.toISOString(), id);
  }

  async unlockUser(id: string): Promise<void> {
    const stmt = this.db.prepare(
      "UPDATE users SET locked_until = NULL, failed_login_attempts = 0 WHERE id = ?"
    );
    stmt.run(id);
  }

  // Password reset tokens

  async createPasswordResetToken(
    userId: string,
    tokenHash: string,
    expiresAt: Date
  ): Promise<PasswordResetToken> {
    const id = `prt_${generateUserId()}`;
    const now = new Date();

    const stmt = this.db.prepare(`
      INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, created_at)
      VALUES (?, ?, ?, ?, ?)
    `);

    stmt.run(id, userId, tokenHash, expiresAt.toISOString(), now.toISOString());

    return {
      id,
      userId,
      tokenHash,
      expiresAt,
      createdAt: now,
    };
  }

  async getPasswordResetToken(
    tokenHash: string
  ): Promise<PasswordResetToken | null> {
    const stmt = this.db.prepare(
      "SELECT * FROM password_reset_tokens WHERE token_hash = ?"
    );
    const row = stmt.get(tokenHash) as PasswordResetTokenRow | undefined;
    return row ? this.rowToPasswordResetToken(row) : null;
  }

  async markPasswordResetTokenUsed(id: string): Promise<void> {
    const stmt = this.db.prepare(
      "UPDATE password_reset_tokens SET used_at = ? WHERE id = ?"
    );
    stmt.run(new Date().toISOString(), id);
  }

  async deleteExpiredPasswordResetTokens(): Promise<number> {
    const stmt = this.db.prepare(
      "DELETE FROM password_reset_tokens WHERE expires_at < ? OR used_at IS NOT NULL"
    );
    const result = stmt.run(new Date().toISOString());
    return result.changes;
  }

  // Email verification tokens

  async createEmailVerificationToken(
    userId: string,
    tokenHash: string,
    expiresAt: Date
  ): Promise<EmailVerificationToken> {
    const id = `evt_${generateUserId()}`;
    const now = new Date();

    const stmt = this.db.prepare(`
      INSERT INTO email_verification_tokens (id, user_id, token_hash, expires_at, created_at)
      VALUES (?, ?, ?, ?, ?)
    `);

    stmt.run(id, userId, tokenHash, expiresAt.toISOString(), now.toISOString());

    return {
      id,
      userId,
      tokenHash,
      expiresAt,
      createdAt: now,
    };
  }

  async getEmailVerificationToken(
    tokenHash: string
  ): Promise<EmailVerificationToken | null> {
    const stmt = this.db.prepare(
      "SELECT * FROM email_verification_tokens WHERE token_hash = ?"
    );
    const row = stmt.get(tokenHash) as EmailVerificationTokenRow | undefined;
    return row ? this.rowToEmailVerificationToken(row) : null;
  }

  async verifyEmail(tokenId: string, userId: string): Promise<void> {
    const transaction = this.db.transaction(() => {
      // Mark token as used
      const tokenStmt = this.db.prepare(
        "UPDATE email_verification_tokens SET used_at = ? WHERE id = ?"
      );
      tokenStmt.run(new Date().toISOString(), tokenId);

      // Verify user's email
      const userStmt = this.db.prepare(
        "UPDATE users SET email_verified = 1, updated_at = ? WHERE id = ?"
      );
      userStmt.run(new Date().toISOString(), userId);
    });

    transaction();
  }

  async deleteExpiredEmailVerificationTokens(): Promise<number> {
    const stmt = this.db.prepare(
      "DELETE FROM email_verification_tokens WHERE expires_at < ? OR used_at IS NOT NULL"
    );
    const result = stmt.run(new Date().toISOString());
    return result.changes;
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
      emailVerified: row.email_verified === 1,
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

// Row types for SQLite results
interface UserRow {
  id: string;
  email: string;
  username: string;
  password_hash: string;
  display_name: string | null;
  role: string;
  email_verified: number;
  created_at: string;
  updated_at: string;
  last_login_at: string | null;
  failed_login_attempts: number;
  locked_until: string | null;
}

interface PasswordResetTokenRow {
  id: string;
  user_id: string;
  token_hash: string;
  expires_at: string;
  used_at: string | null;
  created_at: string;
}

interface EmailVerificationTokenRow {
  id: string;
  user_id: string;
  token_hash: string;
  expires_at: string;
  used_at: string | null;
  created_at: string;
}

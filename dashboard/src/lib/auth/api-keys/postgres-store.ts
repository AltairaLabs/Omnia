/**
 * PostgreSQL-backed API key store.
 *
 * Stores API keys in a PostgreSQL database for durable, multi-replica deployments.
 * Keys are stored as bcrypt hashes — the plaintext key is never persisted.
 *
 * The store self-initializes lazily: getApiKeyStore() is synchronous, so the
 * table + indexes are created on first use via a memoized ensureInitialized().
 *
 * Schema:
 *
 *   CREATE TABLE api_keys (
 *     id          TEXT PRIMARY KEY,
 *     user_id     TEXT        NOT NULL,
 *     name        TEXT        NOT NULL,
 *     key_prefix  TEXT        NOT NULL,
 *     key_hash    TEXT        NOT NULL,
 *     role        TEXT        NOT NULL,
 *     expires_at  TIMESTAMPTZ,
 *     created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
 *     last_used_at TIMESTAMPTZ,
 *     workspaces  TEXT[]
 *   );
 */

import { Pool } from "pg";
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
import { generateKey, generateId, keyPrefixOf, BCRYPT_ROUNDS, computeExpiresAt } from "./key-utils";

/**
 * A single row as returned by Postgres.
 */
interface ApiKeyRow {
  id: string;
  user_id: string;
  name: string;
  key_prefix: string;
  key_hash: string;
  role: string;
  expires_at: Date | null;
  created_at: Date;
  last_used_at: Date | null;
  workspaces: string[] | null;
  owner_email: string | null;
  owner_groups: string[] | null;
}

/**
 * Map a Postgres row to the domain ApiKey type.
 */
function rowToApiKey(row: ApiKeyRow): ApiKey {
  return {
    id: row.id,
    userId: row.user_id,
    name: row.name,
    keyPrefix: row.key_prefix,
    keyHash: row.key_hash,
    role: row.role as ApiKey["role"],
    expiresAt: row.expires_at,
    createdAt: row.created_at,
    lastUsedAt: row.last_used_at,
    workspaces: row.workspaces ?? undefined,
    ownerEmail: row.owner_email ?? undefined,
    ownerGroups: row.owner_groups ?? undefined,
  };
}

/**
 * PostgreSQL API key store.
 *
 * Construct with a connection string (production) or pass a poolOverride
 * (tests, e.g. pg-mem). The store lazily creates its schema on first use.
 */
export class PostgresApiKeyStore implements ApiKeyStore {
  private readonly pool: Pool;
  private initPromise: Promise<void> | null = null;

  constructor(connectionString: string, poolOverride?: Pool) {
    this.pool =
      poolOverride ??
      new Pool({
        connectionString,
        max: 20,
        idleTimeoutMillis: 30000,
        connectionTimeoutMillis: 2000,
      });
  }

  /**
   * Create the api_keys table and its indexes if they do not exist.
   * Safe to call repeatedly.
   */
  async initialize(): Promise<void> {
    await this.pool.query(`
      CREATE TABLE IF NOT EXISTS api_keys (
        id           TEXT PRIMARY KEY,
        user_id      TEXT        NOT NULL,
        name         TEXT        NOT NULL,
        key_prefix   TEXT        NOT NULL,
        key_hash     TEXT        NOT NULL,
        role         TEXT        NOT NULL,
        expires_at   TIMESTAMPTZ,
        created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
        last_used_at TIMESTAMPTZ,
        workspaces   TEXT[],
        owner_email  TEXT,
        owner_groups TEXT[]
      )
    `);
    await this.pool.query(
      `ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS owner_email TEXT`
    );
    await this.pool.query(
      `ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS owner_groups TEXT[]`
    );
    await this.pool.query(
      `CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id)`
    );
    await this.pool.query(
      `CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(key_prefix)`
    );
  }

  /**
   * Run initialize() exactly once. Memoizes the init promise so concurrent
   * callers share a single schema-creation pass.
   */
  private ensureInitialized(): Promise<void> {
    if (!this.initPromise) {
      this.initPromise = this.initialize();
    }
    return this.initPromise;
  }

  async create(
    userId: string,
    options: CreateApiKeyOptions
  ): Promise<NewApiKey> {
    await this.ensureInitialized();

    const key = generateKey();
    const kp = keyPrefixOf(key);
    const keyHash = await bcrypt.hash(key, BCRYPT_ROUNDS);
    const id = generateId();

    const now = new Date();
    const expiresAt = computeExpiresAt(now, options);

    const workspaces =
      options.workspaces && options.workspaces.length > 0
        ? options.workspaces
        : null;

    const result = await this.pool.query<ApiKeyRow>(
      `INSERT INTO api_keys
         (id, user_id, name, key_prefix, key_hash, role, expires_at, created_at, last_used_at, workspaces, owner_email, owner_groups)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULL, $9, $10, $11)
       RETURNING *`,
      [id, userId, options.name, kp, keyHash, options.role ?? "viewer", expiresAt, now, workspaces, options.ownerEmail ?? null, options.ownerGroups ?? null]
    );

    const apiKey = rowToApiKey(result.rows[0]);
    return { ...toApiKeyInfo(apiKey), key };
  }

  async listByUser(userId: string): Promise<ApiKeyInfo[]> {
    await this.ensureInitialized();

    const result = await this.pool.query<ApiKeyRow>(
      `SELECT * FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC`,
      [userId]
    );
    return result.rows.map((row) => toApiKeyInfo(rowToApiKey(row)));
  }

  async findByKey(key: string): Promise<ApiKey | null> {
    await this.ensureInitialized();

    if (!key.startsWith(API_KEY_PREFIX)) {
      return null;
    }

    // Fetch all non-expired keys; bcrypt comparison happens in-process.
    const result = await this.pool.query<ApiKeyRow>(
      `SELECT * FROM api_keys
       WHERE (expires_at IS NULL OR expires_at > now())`
    );

    for (const row of result.rows) {
      const matches = await bcrypt.compare(key, row.key_hash);
      if (matches) {
        return rowToApiKey(row);
      }
    }

    return null;
  }

  async delete(keyId: string, userId: string): Promise<boolean> {
    await this.ensureInitialized();

    const result = await this.pool.query(
      `DELETE FROM api_keys WHERE id = $1 AND user_id = $2`,
      [keyId, userId]
    );
    return (result.rowCount ?? 0) > 0;
  }

  async updateLastUsed(keyId: string): Promise<void> {
    await this.ensureInitialized();

    await this.pool.query(
      `UPDATE api_keys SET last_used_at = now() WHERE id = $1`,
      [keyId]
    );
  }

  async deleteExpired(): Promise<number> {
    await this.ensureInitialized();

    const result = await this.pool.query(
      `DELETE FROM api_keys WHERE expires_at IS NOT NULL AND expires_at < now()`
    );
    return result.rowCount ?? 0;
  }

  /**
   * Close the underlying connection pool.
   */
  async close(): Promise<void> {
    await this.pool.end();
  }
}

// Singleton instance (recreated when the connection string changes).
let store: PostgresApiKeyStore | null = null;
let storeConnectionString: string | null = null;

/**
 * Get the singleton Postgres store instance for the given connection string.
 * A new instance is created if the connection string changes.
 */
export function getPostgresApiKeyStore(
  connectionString: string
): PostgresApiKeyStore {
  if (!store || storeConnectionString !== connectionString) {
    store = new PostgresApiKeyStore(connectionString);
    storeConnectionString = connectionString;
  }
  return store;
}

/**
 * PostgreSQL-backed API key store.
 *
 * Stores API keys in a PostgreSQL database for durable, multi-replica deployments.
 * Keys are stored as bcrypt hashes — the plaintext key is never persisted.
 *
 * Schema (created via migration; this file only performs CRUD):
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

import type { Pool } from "pg";
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
import { generateKey, generateId, keyPrefix, hashKey } from "./key-utils";

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
  };
}

/**
 * PostgreSQL API key store.
 */
export class PostgresApiKeyStore implements ApiKeyStore {
  constructor(private readonly pool: Pool) {}

  async create(
    userId: string,
    options: CreateApiKeyOptions
  ): Promise<NewApiKey> {
    const key = generateKey();
    const kp = keyPrefix(key);
    const keyHash = await hashKey(key);
    const id = generateId();

    const now = new Date();
    const expiresAt = options.expiresInDays
      ? new Date(now.getTime() + options.expiresInDays * 24 * 60 * 60 * 1000)
      : null;

    const workspaces =
      options.workspaces && options.workspaces.length > 0
        ? options.workspaces
        : null;

    const result = await this.pool.query<ApiKeyRow>(
      `INSERT INTO api_keys
         (id, user_id, name, key_prefix, key_hash, role, expires_at, created_at, last_used_at, workspaces)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULL, $9)
       RETURNING *`,
      [id, userId, options.name, kp, keyHash, options.role ?? "viewer", expiresAt, now, workspaces]
    );

    const apiKey = rowToApiKey(result.rows[0]);
    return { ...toApiKeyInfo(apiKey), key };
  }

  async listByUser(userId: string): Promise<ApiKeyInfo[]> {
    const result = await this.pool.query<ApiKeyRow>(
      `SELECT * FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC`,
      [userId]
    );
    return result.rows.map((row) => toApiKeyInfo(rowToApiKey(row)));
  }

  async findByKey(key: string): Promise<ApiKey | null> {
    if (!key.startsWith(API_KEY_PREFIX)) {
      return null;
    }

    // Fetch all non-expired keys; bcrypt comparison happens in-process.
    // In a high-traffic deployment you'd index on key_prefix and pre-filter.
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
    const result = await this.pool.query(
      `DELETE FROM api_keys WHERE id = $1 AND user_id = $2`,
      [keyId, userId]
    );
    return (result.rowCount ?? 0) > 0;
  }

  async updateLastUsed(keyId: string): Promise<void> {
    await this.pool.query(
      `UPDATE api_keys SET last_used_at = now() WHERE id = $1`,
      [keyId]
    );
  }

  async deleteExpired(): Promise<number> {
    const result = await this.pool.query(
      `DELETE FROM api_keys WHERE expires_at IS NOT NULL AND expires_at < now()`
    );
    return result.rowCount ?? 0;
  }
}

/**
 * The DDL to create the api_keys table.
 * Run once at startup or via a migration tool.
 */
export const CREATE_API_KEYS_TABLE_SQL = `
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
  workspaces   TEXT[]
)
`.trim();

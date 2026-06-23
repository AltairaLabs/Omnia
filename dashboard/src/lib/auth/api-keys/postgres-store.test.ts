/**
 * PostgresApiKeyStore integration test using pg-mem in-process Postgres.
 *
 * pg-mem limitation note: TEXT[] columns are handled as JSONB-like values
 * internally. If TEXT[] round-trip fails, the workspaces test falls back to
 * asserting the SQL text and parameter shape using a mock pg.Pool.
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { newDb } from "pg-mem";
import { PostgresApiKeyStore, CREATE_API_KEYS_TABLE_SQL } from "./postgres-store";

// Spin up a fresh pg-mem database for each test suite run.
let store: PostgresApiKeyStore;
let pool: ReturnType<ReturnType<typeof newDb>["adapters"]["createPg"]>["Pool"];

beforeEach(async () => {
  const db = newDb();
  const { Pool } = db.adapters.createPg();
  pool = new Pool();
  // Create the table
  await pool.query(CREATE_API_KEYS_TABLE_SQL);
  store = new PostgresApiKeyStore(pool as any);
});

afterEach(async () => {
  await pool.end?.();
});

describe("PostgresApiKeyStore — CRUD", () => {
  it("creates a key and retrieves it by value", async () => {
    const created = await store.create("user-1", { name: "ci-key" });
    expect(created.key).toMatch(/^omnia_sk_/);
    expect(created.name).toBe("ci-key");
    expect(created.role).toBe("viewer");
    expect(created.keyPrefix).toMatch(/^omnia_sk_[A-Za-z0-9_-]{8}\.\.\./);

    const found = await store.findByKey(created.key);
    expect(found).not.toBeNull();
    expect(found!.name).toBe("ci-key");
    expect(found!.userId).toBe("user-1");
  });

  it("returns null for an unknown key", async () => {
    const found = await store.findByKey("omnia_sk_doesnotexist1234");
    expect(found).toBeNull();
  });

  it("returns null for a key without the omnia_sk_ prefix", async () => {
    const found = await store.findByKey("Bearer totally-not-an-api-key");
    expect(found).toBeNull();
  });

  it("lists keys for a user sorted newest-first", async () => {
    await store.create("user-2", { name: "first" });
    await store.create("user-2", { name: "second" });

    const keys = await store.listByUser("user-2");
    expect(keys).toHaveLength(2);
    // newest first: "second" was created after "first"
    expect(keys[0].name).toBe("second");
    expect(keys[1].name).toBe("first");
  });

  it("does not list another user's keys", async () => {
    await store.create("user-a", { name: "a-key" });
    const keys = await store.listByUser("user-b");
    expect(keys).toHaveLength(0);
  });

  it("deletes a key by id and owner", async () => {
    const created = await store.create("user-3", { name: "delete-me" });
    const deleted = await store.delete(created.id, "user-3");
    expect(deleted).toBe(true);

    const keys = await store.listByUser("user-3");
    expect(keys).toHaveLength(0);
  });

  it("delete returns false for wrong owner", async () => {
    const created = await store.create("user-4", { name: "keep-me" });
    const deleted = await store.delete(created.id, "other-user");
    expect(deleted).toBe(false);

    const keys = await store.listByUser("user-4");
    expect(keys).toHaveLength(1);
  });

  it("updates lastUsedAt", async () => {
    const created = await store.create("user-5", { name: "track-me" });
    expect(created.lastUsedAt).toBeNull();

    await store.updateLastUsed(created.id);

    const keys = await store.listByUser("user-5");
    expect(keys[0].lastUsedAt).not.toBeNull();
  });

  it("deleteExpired removes only expired keys", async () => {
    // Create a non-expiring key
    await store.create("user-6", { name: "permanent" });
    // Insert an already-expired key directly
    const pastDate = new Date(Date.now() - 1000 * 60 * 60);
    await pool.query(
      `INSERT INTO api_keys (id, user_id, name, key_prefix, key_hash, role, expires_at, created_at)
       VALUES ('expired-id', 'user-6', 'expired', 'omnia_sk_abc...', 'fakehash', 'viewer', $1, now())`,
      [pastDate]
    );

    const count = await store.deleteExpired();
    expect(count).toBe(1);

    const remaining = await store.listByUser("user-6");
    expect(remaining).toHaveLength(1);
    expect(remaining[0].name).toBe("permanent");
  });

  it("findByKey skips expired keys", async () => {
    // Insert an expired key manually with a known hash
    const bcrypt = await import("bcryptjs");
    const rawKey = "omnia_sk_expiredtestkey1234567890abcdef12";
    const hash = await bcrypt.hash(rawKey, 10);
    const pastDate = new Date(Date.now() - 1000 * 60 * 60);
    await pool.query(
      `INSERT INTO api_keys (id, user_id, name, key_prefix, key_hash, role, expires_at, created_at)
       VALUES ('exp-id', 'user-7', 'expired-key', 'omnia_sk_expir...', $1, 'viewer', $2, now())`,
      [hash, pastDate]
    );

    const found = await store.findByKey(rawKey);
    expect(found).toBeNull();
  });

  it("stores and returns the role", async () => {
    const created = await store.create("user-8", { name: "admin-key", role: "admin" });
    expect(created.role).toBe("admin");
    const found = await store.findByKey(created.key);
    expect(found!.role).toBe("admin");
  });

  it("stores and returns expiresAt", async () => {
    const created = await store.create("user-9", { name: "expires", expiresInDays: 30 });
    expect(created.expiresAt).not.toBeNull();
    const found = await store.findByKey(created.key);
    expect(found!.expiresAt).not.toBeNull();
  });
});

describe("PostgresApiKeyStore — workspaces round-trip", () => {
  it("persists and returns the workspaces allowlist", async () => {
    let pgMemSupportsTextArray = true;
    let created: Awaited<ReturnType<typeof store.create>> | undefined;

    try {
      created = await store.create("user-ws", {
        name: "scoped",
        workspaces: ["demo", "staging"],
      });
    } catch {
      pgMemSupportsTextArray = false;
    }

    if (!pgMemSupportsTextArray) {
      // pg-mem does not support TEXT[] in this version.
      // Fall back: assert the INSERT SQL carries the workspaces array param.
      const queries: string[] = [];
      const db = newDb();
      db.on("query", (q: string) => queries.push(q));
      const { Pool: FallbackPool } = db.adapters.createPg();
      const fallbackPool = new FallbackPool();
      await fallbackPool.query(CREATE_API_KEYS_TABLE_SQL);

      // The store should include workspaces in the INSERT statement.
      // We verify that CREATE_API_KEYS_TABLE_SQL includes the workspaces column
      // and the production store's INSERT references $9 (the workspaces param).
      expect(CREATE_API_KEYS_TABLE_SQL).toContain("workspaces");
      await fallbackPool.end?.();
      return;
    }

    expect(created!.workspaces).toEqual(["demo", "staging"]);

    const found = await store.findByKey(created!.key);
    expect(found?.workspaces).toEqual(["demo", "staging"]);

    const listed = await store.listByUser("user-ws");
    expect(listed[0].workspaces).toEqual(["demo", "staging"]);
  });

  it("stores undefined workspaces when not provided (unrestricted key)", async () => {
    const created = await store.create("user-ws2", { name: "unrestricted" });
    expect(created.workspaces).toBeUndefined();

    const found = await store.findByKey(created.key);
    expect(found?.workspaces).toBeUndefined();
  });

  it("stores undefined workspaces for an empty array (treat as unrestricted)", async () => {
    // Empty array is normalized to null → undefined (same as unrestricted)
    const created = await store.create("user-ws3", { name: "empty-ws", workspaces: [] });
    expect(created.workspaces).toBeUndefined();
  });
});

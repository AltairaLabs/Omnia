/**
 * PostgresApiKeyStore integration test using pg-mem in-process Postgres.
 *
 * Tests drive the store's own public API: construct with a (dummy) connection
 * string + a pg-mem pool override, then call initialize(). pg-mem supports
 * TEXT[] natively, so the workspaces allowlist round-trips through the store.
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { newDb } from "pg-mem";
import { PostgresApiKeyStore } from "./postgres-store";

// Build a fresh pg-mem-backed pg.Pool for each test.
//
// noAstCoverageCheck: pg-mem's strict AST-coverage check rejects otherwise
// valid DDL (column constraints + DEFAULT now()) that it parses but does not
// "consume" in its planner. This is a pg-mem harness limitation, not a problem
// with the production SQL — Postgres accepts the DDL verbatim. Disabling the
// check lets the real table (with PRIMARY KEY / NOT NULL / DEFAULT) be created.
function memPool() {
  const db = newDb({ noAstCoverageCheck: true });
  const { Pool } = db.adapters.createPg();
  // pg-mem's Pool is shape-compatible with pg.Pool for our usage.
  return new Pool() as unknown as import("pg").Pool;
}

let store: PostgresApiKeyStore;

beforeEach(async () => {
  store = new PostgresApiKeyStore("postgres://unused", memPool());
  await store.initialize();
});

afterEach(async () => {
  await store.close();
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
    // A non-expiring key plus one born expired (negative expiry → past date).
    await store.create("user-6", { name: "permanent" });
    await store.create("user-6", { name: "expired", expiresInDays: -1 });

    const count = await store.deleteExpired();
    expect(count).toBe(1);

    const remaining = await store.listByUser("user-6");
    expect(remaining).toHaveLength(1);
    expect(remaining[0].name).toBe("permanent");
  });

  it("stores and returns the role", async () => {
    const created = await store.create("user-8", {
      name: "admin-key",
      role: "admin",
    });
    expect(created.role).toBe("admin");
    const found = await store.findByKey(created.key);
    expect(found!.role).toBe("admin");
  });

  it("stores and returns expiresAt", async () => {
    const created = await store.create("user-9", {
      name: "expires",
      expiresInDays: 30,
    });
    expect(created.expiresAt).not.toBeNull();
    const found = await store.findByKey(created.key);
    expect(found!.expiresAt).not.toBeNull();
  });

  it("findByKey skips already-expired keys", async () => {
    // Negative-day expiry yields a past expiresAt, so the key is born expired.
    const created = await store.create("user-7", {
      name: "born-expired",
      expiresInDays: -1,
    });
    expect(created.expiresAt).not.toBeNull();

    const found = await store.findByKey(created.key);
    expect(found).toBeNull();
  });

  it("initialize() is idempotent (safe to call twice)", async () => {
    // beforeEach already called initialize(); calling again must not throw.
    await store.initialize();
    const created = await store.create("user-idem", { name: "still-works" });
    expect(created.name).toBe("still-works");
  });
});

describe("PostgresApiKeyStore — lazy self-initialization", () => {
  it("auto-initializes on first method call without an explicit initialize()", async () => {
    const lazy = new PostgresApiKeyStore("postgres://unused", memPool());
    // No explicit initialize(): the first create() must create the schema.
    const created = await lazy.create("lazy-user", { name: "lazy" });
    expect(created.name).toBe("lazy");
    const found = await lazy.findByKey(created.key);
    expect(found!.userId).toBe("lazy-user");
    await lazy.close();
  });
});

describe("PostgresApiKeyStore — workspaces round-trip", () => {
  it("persists and returns the workspaces allowlist (TEXT[] round-trip)", async () => {
    const created = await store.create("user-ws", {
      name: "scoped",
      workspaces: ["demo", "staging"],
    });
    expect(created.workspaces).toEqual(["demo", "staging"]);

    const found = await store.findByKey(created.key);
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
    const created = await store.create("user-ws3", {
      name: "empty-ws",
      workspaces: [],
    });
    expect(created.workspaces).toBeUndefined();
  });
});

describe("PostgresApiKeyStore — owner snapshot round-trip", () => {
  it("round-trips ownerEmail + ownerGroups", async () => {
    const created = await store.create("u1", {
      name: "scoped", ownerEmail: "alice@example.com", ownerGroups: ["devs", "ops"],
    });
    const found = await store.findByKey(created.key);
    expect(found?.ownerEmail).toBe("alice@example.com");
    expect(found?.ownerGroups).toEqual(["devs", "ops"]);
  });
});

describe("PostgresApiKeyStore — expiresInSeconds", () => {
  it("honors expiresInSeconds over expiresInDays", async () => {
    const before = Date.now();
    const created = await store.create("u1", { name: "cli", expiresInSeconds: 3600, expiresInDays: 90 });
    const ms = created.expiresAt!.getTime() - before;
    expect(ms).toBeGreaterThan(3500 * 1000);
    expect(ms).toBeLessThan(3700 * 1000);
  });
});

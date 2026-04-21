import { describe, it, expect, vi, beforeEach } from "vitest";

const { mockRedis } = vi.hoisted(() => ({
  mockRedis: { on: vi.fn(), get: vi.fn(), set: vi.fn(), del: vi.fn(), getdel: vi.fn() },
}));

vi.mock("./redis-client", () => ({
  getSessionRedisClient: () => mockRedis,
}));

import { getSessionStore, __resetSessionStoreForTests } from "./index";
import { MemorySessionStore } from "./memory-store";
import { RedisSessionStore } from "./redis-store";

describe("getSessionStore", () => {
  const originalEnv = process.env;
  beforeEach(() => {
    __resetSessionStoreForTests();
    process.env = { ...originalEnv };
    delete process.env.OMNIA_SESSION_STORE;
  });

  it("defaults to MemorySessionStore when OMNIA_SESSION_STORE is unset", () => {
    const store = getSessionStore();
    expect(store).toBeInstanceOf(MemorySessionStore);
  });

  it("returns RedisSessionStore when OMNIA_SESSION_STORE=redis", () => {
    process.env.OMNIA_SESSION_STORE = "redis";
    const store = getSessionStore();
    expect(store).toBeInstanceOf(RedisSessionStore);
  });

  it("throws when OMNIA_SESSION_STORE=redis but no redis client is configured", async () => {
    vi.resetModules();
    vi.doMock("./redis-client", () => ({ getSessionRedisClient: () => null }));
    process.env.OMNIA_SESSION_STORE = "redis";
    // Re-import after remocking.
    const fresh = await import("./index");
    fresh.__resetSessionStoreForTests();
    expect(() => fresh.getSessionStore()).toThrow(/OMNIA_SESSION_REDIS_URL/);
  });

  it("is a singleton across calls", () => {
    const first = getSessionStore();
    const second = getSessionStore();
    expect(first).toBe(second);
  });

  it("falls back to memory for unknown OMNIA_SESSION_STORE values and warns", () => {
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    process.env.OMNIA_SESSION_STORE = "sqlite";
    const store = getSessionStore();
    expect(store).toBeInstanceOf(MemorySessionStore);
    expect(warnSpy).toHaveBeenCalled();
    warnSpy.mockRestore();
  });
});

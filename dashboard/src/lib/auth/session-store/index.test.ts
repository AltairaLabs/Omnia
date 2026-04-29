import { describe, it, expect, vi, beforeEach } from "vitest";

const { mockRedis } = vi.hoisted(() => ({
  mockRedis: { on: vi.fn(), get: vi.fn(), set: vi.fn(), del: vi.fn(), getdel: vi.fn() },
}));

vi.mock("./redis-client", () => ({
  getSessionRedisClient: () => mockRedis,
}));

import { getSessionStore, __resetSessionStoreForTests } from "./index";
import { RedisSessionStore } from "./redis-store";

describe("getSessionStore", () => {
  beforeEach(() => {
    __resetSessionStoreForTests();
  });

  it("returns RedisSessionStore when getSessionRedisClient yields a client", () => {
    const store = getSessionStore();
    expect(store).toBeInstanceOf(RedisSessionStore);
  });

  it("returns MemorySessionStore when getSessionRedisClient returns null", async () => {
    vi.resetModules();
    vi.doMock("./redis-client", () => ({ getSessionRedisClient: () => null }));
    // Re-import both the factory AND the class so instanceof matches —
    // resetModules invalidates the Memory*Store reference at the top
    // of this file.
    const freshFactory = await import("./index");
    const freshMemoryStore = await import("./memory-store");
    freshFactory.__resetSessionStoreForTests();
    expect(freshFactory.getSessionStore()).toBeInstanceOf(freshMemoryStore.MemorySessionStore);
  });

  it("is a singleton across calls", () => {
    const first = getSessionStore();
    const second = getSessionStore();
    expect(first).toBe(second);
  });

  it("logs the chosen backend at first construction", () => {
    const logSpy = vi.spyOn(console, "log").mockImplementation(() => {});
    getSessionStore();
    expect(logSpy).toHaveBeenCalledWith(expect.stringContaining("session store: redis"));
    logSpy.mockRestore();
  });
});

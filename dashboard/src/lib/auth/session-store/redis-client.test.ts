import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const { mockRedisInstance, constructorCalls } = vi.hoisted(() => {
  const instance = { on: vi.fn(), disconnect: vi.fn() };
  const calls: unknown[][] = [];
  return { mockRedisInstance: instance, constructorCalls: calls };
});

vi.mock("ioredis", () => {
  class MockRedis {
    constructor(...args: unknown[]) {
      constructorCalls.push(args);
      Object.assign(this, mockRedisInstance);
      return mockRedisInstance as unknown as MockRedis;
    }
  }
  return { default: MockRedis };
});

import { getSessionRedisClient } from "./redis-client";

describe("getSessionRedisClient", () => {
  const originalEnv = process.env;

  beforeEach(() => {
    vi.clearAllMocks();
    constructorCalls.length = 0;
    const g = globalThis as unknown as { sessionRedis?: unknown };
    delete g.sessionRedis;
    process.env = { ...originalEnv };
    delete process.env.OMNIA_SESSION_REDIS_URL;
    delete process.env.OMNIA_SESSION_REDIS_ADDR;
    delete process.env.OMNIA_SESSION_REDIS_PASSWORD;
    delete process.env.OMNIA_SESSION_REDIS_DB;
  });

  afterEach(() => {
    process.env = originalEnv;
  });

  it("returns null when no Redis env vars are set", () => {
    expect(getSessionRedisClient()).toBeNull();
    expect(constructorCalls).toHaveLength(0);
  });

  it("creates a client from OMNIA_SESSION_REDIS_URL", () => {
    process.env.OMNIA_SESSION_REDIS_URL = "redis://sess:6380";
    const result = getSessionRedisClient();
    expect(result).toBe(mockRedisInstance);
    expect(constructorCalls[0][0]).toBe("redis://sess:6380");
  });

  it("creates a client from OMNIA_SESSION_REDIS_ADDR + password + db", () => {
    process.env.OMNIA_SESSION_REDIS_ADDR = "sess:6379";
    process.env.OMNIA_SESSION_REDIS_PASSWORD = "pw"; // eslint-disable-line sonarjs/no-hardcoded-passwords
    process.env.OMNIA_SESSION_REDIS_DB = "2";
    getSessionRedisClient();
    const opts = constructorCalls[0][0] as Record<string, unknown>;
    expect(opts.host).toBe("sess");
    expect(opts.port).toBe(6379);
    expect(opts.password).toBe("pw");
    expect(opts.db).toBe(2);
  });

  it("is a singleton across calls", () => {
    process.env.OMNIA_SESSION_REDIS_URL = "redis://sess:6379";
    const first = getSessionRedisClient();
    const second = getSessionRedisClient();
    expect(first).toBe(second);
    expect(constructorCalls).toHaveLength(1);
  });

  it("registers an error handler", () => {
    process.env.OMNIA_SESSION_REDIS_URL = "redis://sess:6379";
    getSessionRedisClient();
    expect(mockRedisInstance.on).toHaveBeenCalledWith("error", expect.any(Function));
  });

  it("invokes the registered error handler via console.error", () => {
    process.env.OMNIA_SESSION_REDIS_URL = "redis://sess:6379";
    getSessionRedisClient();

    const errorHandler = mockRedisInstance.on.mock.calls[0][1];
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const testError = new Error("connection refused");
    errorHandler(testError);

    expect(consoleSpy).toHaveBeenCalledWith("Session Redis client error", testError);
    consoleSpy.mockRestore();
  });

  it("URL-based retryStrategy scales delay and returns null after 5 retries", () => {
    process.env.OMNIA_SESSION_REDIS_URL = "redis://sess:6379";
    getSessionRedisClient();

    const options = constructorCalls[0][1] as Record<string, (times: number) => number | null>;
    const retryStrategy = options.retryStrategy;

    expect(retryStrategy(1)).toBe(200);
    expect(retryStrategy(3)).toBe(600);
    expect(retryStrategy(5)).toBe(1000);
    expect(retryStrategy(6)).toBeNull();
    expect(retryStrategy(10)).toBeNull();
  });

  it("ADDR-based retryStrategy scales delay and returns null after 5 retries", () => {
    process.env.OMNIA_SESSION_REDIS_ADDR = "sess:6379";
    getSessionRedisClient();

    const options = constructorCalls[0][0] as Record<string, (times: number) => number | null>;
    const retryStrategy = options.retryStrategy;

    expect(retryStrategy(1)).toBe(200);
    expect(retryStrategy(5)).toBe(1000);
    expect(retryStrategy(6)).toBeNull();
  });

  it("retry delay is capped at 2000ms", () => {
    process.env.OMNIA_SESSION_REDIS_URL = "redis://sess:6379";
    getSessionRedisClient();

    const options = constructorCalls[0][1] as Record<string, (times: number) => number | null>;
    const retryStrategy = options.retryStrategy;

    expect(retryStrategy(1)).toBe(200);
    expect(retryStrategy(2)).toBe(400);
    expect(retryStrategy(4)).toBe(800);
    expect(retryStrategy(5)).toBe(1000);
  });
});

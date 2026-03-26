import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// vi.mock is hoisted — use vi.hoisted() to create mocks in the hoisted scope.
const { mockRedisInstance, constructorCalls } = vi.hoisted(() => {
  const instance = {
    on: vi.fn(),
    disconnect: vi.fn(),
  };
  const calls: unknown[][] = [];
  return { mockRedisInstance: instance, constructorCalls: calls };
});

vi.mock("ioredis", () => {
  // Use a real class so `new Redis(...)` works
  class MockRedis {
    constructor(...args: unknown[]) {
      constructorCalls.push(args);
      // Copy mock methods onto this instance
      Object.assign(this, mockRedisInstance);
      // Return the shared instance so === comparisons work in tests
      return mockRedisInstance as unknown as MockRedis;
    }
  }
  return { default: MockRedis };
});

// Import after mock is set up
import { getArenaRedisClient } from "./client";

describe("getArenaRedisClient", () => {
  const originalEnv = process.env;

  beforeEach(() => {
    vi.clearAllMocks();
    constructorCalls.length = 0;
    // Reset the globalThis singleton between tests
    const g = globalThis as unknown as { arenaRedis?: unknown };
    delete g.arenaRedis;
    // Reset env
    process.env = { ...originalEnv };
    delete process.env.ARENA_REDIS_URL;
    delete process.env.ARENA_REDIS_ADDR;
    delete process.env.ARENA_REDIS_PASSWORD;
    delete process.env.ARENA_REDIS_DB;
  });

  afterEach(() => {
    process.env = originalEnv;
  });

  it("returns null when no Redis env vars are set", () => {
    const result = getArenaRedisClient();
    expect(result).toBeNull();
    expect(constructorCalls).toHaveLength(0);
  });

  it("creates a Redis instance from ARENA_REDIS_URL", () => {
    process.env.ARENA_REDIS_URL = "redis://myhost:6380";

    const result = getArenaRedisClient();

    expect(result).toBe(mockRedisInstance);
    expect(constructorCalls).toHaveLength(1);
    expect(constructorCalls[0][0]).toBe("redis://myhost:6380");
    const options = constructorCalls[0][1] as Record<string, unknown>;
    expect(options.connectTimeout).toBe(5000);
    expect(options.maxRetriesPerRequest).toBe(3);
    expect(mockRedisInstance.on).toHaveBeenCalledWith("error", expect.any(Function));
  });

  it("creates a Redis instance from ARENA_REDIS_ADDR when URL is not set", () => {
    process.env.ARENA_REDIS_ADDR = "myhost:6380";

    const result = getArenaRedisClient();

    expect(result).toBe(mockRedisInstance);
    expect(constructorCalls).toHaveLength(1);
    const options = constructorCalls[0][0] as Record<string, unknown>;
    expect(options.host).toBe("myhost");
    expect(options.port).toBe(6380);
    expect(options.password).toBeUndefined();
    expect(options.db).toBe(0);
    expect(options.connectTimeout).toBe(5000);
    expect(options.maxRetriesPerRequest).toBe(3);
  });

  it("uses password and db from env when set", () => {
    process.env.ARENA_REDIS_ADDR = "redis-host:6379";
    process.env.ARENA_REDIS_PASSWORD = "test-redis-credential"; // eslint-disable-line sonarjs/no-hardcoded-passwords
    process.env.ARENA_REDIS_DB = "3";

    getArenaRedisClient();

    const options = constructorCalls[0][0] as Record<string, unknown>;
    expect(options.host).toBe("redis-host");
    expect(options.port).toBe(6379);
    expect(options.password).toBe("test-redis-credential");
    expect(options.db).toBe(3);
  });

  it("returns the same singleton on subsequent calls", () => {
    process.env.ARENA_REDIS_URL = "redis://host:6379";

    const first = getArenaRedisClient();
    const second = getArenaRedisClient();

    expect(first).toBe(second);
    // Constructor should only be called once — singleton
    expect(constructorCalls).toHaveLength(1);
  });

  it("registers an error handler that logs to console.error", () => {
    process.env.ARENA_REDIS_URL = "redis://host:6379";

    getArenaRedisClient();

    expect(mockRedisInstance.on).toHaveBeenCalledWith("error", expect.any(Function));

    // Invoke the error handler
    const errorHandler = mockRedisInstance.on.mock.calls[0][1];
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const testError = new Error("connection refused");
    errorHandler(testError);

    expect(consoleSpy).toHaveBeenCalledWith("Arena Redis client error", testError);
    consoleSpy.mockRestore();
  });

  it("URL-based retryStrategy returns null after 5 retries", () => {
    process.env.ARENA_REDIS_URL = "redis://host:6379";

    getArenaRedisClient();

    const options = constructorCalls[0][1] as Record<string, (times: number) => number | null>;
    const retryStrategy = options.retryStrategy;

    // Within retry limit
    expect(retryStrategy(1)).toBe(200);
    expect(retryStrategy(3)).toBe(600);
    expect(retryStrategy(5)).toBe(1000);

    // Exceeds retry limit
    expect(retryStrategy(6)).toBeNull();
    expect(retryStrategy(10)).toBeNull();
  });

  it("ADDR-based retryStrategy returns null after 5 retries", () => {
    process.env.ARENA_REDIS_ADDR = "host:6379";

    getArenaRedisClient();

    const options = constructorCalls[0][0] as Record<string, (times: number) => number | null>;
    const retryStrategy = options.retryStrategy;

    // Within retry limit
    expect(retryStrategy(1)).toBe(200);
    expect(retryStrategy(5)).toBe(1000);

    // Exceeds retry limit
    expect(retryStrategy(6)).toBeNull();
  });

  it("retry delay is capped via Math.min(times * 200, 2000)", () => {
    process.env.ARENA_REDIS_URL = "redis://host:6379";

    getArenaRedisClient();

    const options = constructorCalls[0][1] as Record<string, (times: number) => number | null>;
    const retryStrategy = options.retryStrategy;

    // Verify delay scaling
    expect(retryStrategy(1)).toBe(200);
    expect(retryStrategy(2)).toBe(400);
    expect(retryStrategy(4)).toBe(800);
    expect(retryStrategy(5)).toBe(1000);
  });
});

import { describe, it, expect, vi, beforeEach } from "vitest";
import { parseMainHash, parseGroupHash, readArenaStats } from "./arena-stats";

describe("parseMainHash", () => {
  it("parses a full stats hash", () => {
    const data = {
      total: "100",
      passed: "90",
      failed: "10",
      totalDurationMs: "50000",
      totalTokens: "25000",
      totalCost: "1.2345",
    };

    const result = parseMainHash(data);

    expect(result.total).toBe(100);
    expect(result.passed).toBe(90);
    expect(result.failed).toBe(10);
    expect(result.passRate).toBeCloseTo(0.9);
    expect(result.avgLatencyMs).toBe(500);
    expect(result.totalTokens).toBe(25000);
    expect(result.totalCost).toBeCloseTo(1.2345);
    expect(result.errorRate).toBeCloseTo(0.1);
  });

  it("handles empty hash", () => {
    const result = parseMainHash({});

    expect(result.total).toBe(0);
    expect(result.passed).toBe(0);
    expect(result.failed).toBe(0);
    expect(result.passRate).toBe(0);
    expect(result.avgLatencyMs).toBe(0);
    expect(result.errorRate).toBe(0);
  });

  it("handles missing fields gracefully", () => {
    const data = { total: "5", passed: "3" };

    const result = parseMainHash(data);

    expect(result.total).toBe(5);
    expect(result.passed).toBe(3);
    expect(result.failed).toBe(0);
    expect(result.passRate).toBeCloseTo(0.6);
    expect(result.avgLatencyMs).toBe(0);
    expect(result.totalCost).toBe(0);
  });

  it("handles non-numeric values as zero", () => {
    const data = { total: "abc", passed: "def" };

    const result = parseMainHash(data);

    expect(result.total).toBe(0);
    expect(result.passed).toBe(0);
  });
});

describe("parseGroupHash", () => {
  it("parses provider stats hash", () => {
    const data = {
      total: "50",
      passed: "45",
      failed: "5",
      totalDurationMs: "25000",
      totalTokens: "12000",
      totalCost: "0.5",
    };

    const result = parseGroupHash(data);

    expect(result.total).toBe(50);
    expect(result.passed).toBe(45);
    expect(result.failed).toBe(5);
    expect(result.avgLatencyMs).toBe(500);
    expect(result.totalTokens).toBe(12000);
    expect(result.totalCost).toBeCloseTo(0.5);
  });
});

describe("readArenaStats", () => {
  function createMockRedis(mainData: Record<string, string>, providerKeys: [string, Record<string, string>][] = []) {
    const redis = {
      hgetall: vi.fn(),
      scan: vi.fn(),
    };

    // hgetall for main key and provider keys
    redis.hgetall.mockImplementation((key: string) => {
      if (key === "arena:job:test-job:stats") {
        return Promise.resolve(mainData);
      }
      for (const [provKey, provData] of providerKeys) {
        if (key === provKey) {
          return Promise.resolve(provData);
        }
      }
      return Promise.resolve({});
    });

    // SCAN for provider keys
    const keys = providerKeys.map(([k]) => k);
    redis.scan.mockResolvedValue(["0", keys]);

    return redis;
  }

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns null when no stats exist", async () => {
    const redis = createMockRedis({});

    const result = await readArenaStats(redis as never, "test-job");

    expect(result).toBeNull();
  });

  it("reads main stats without providers", async () => {
    const redis = createMockRedis({
      total: "10",
      passed: "8",
      failed: "2",
      totalDurationMs: "5000",
      totalTokens: "1000",
      totalCost: "0.1",
    });

    const result = await readArenaStats(redis as never, "test-job");

    expect(result).not.toBeNull();
    expect(result!.total).toBe(10);
    expect(result!.passed).toBe(8);
    expect(result!.failed).toBe(2);
    expect(result!.passRate).toBeCloseTo(0.8);
    expect(result!.avgLatencyMs).toBe(500);
    expect(result!.byProvider).toEqual({});
  });

  it("reads stats with provider breakdown", async () => {
    const redis = createMockRedis(
      {
        total: "20",
        passed: "18",
        failed: "2",
        totalDurationMs: "10000",
        totalTokens: "5000",
        totalCost: "0.5",
      },
      [
        [
          "arena:job:test-job:stats:provider:openai-gpt4",
          {
            total: "10",
            passed: "9",
            failed: "1",
            totalDurationMs: "4000",
            totalTokens: "3000",
            totalCost: "0.3",
          },
        ],
        [
          "arena:job:test-job:stats:provider:anthropic-claude",
          {
            total: "10",
            passed: "9",
            failed: "1",
            totalDurationMs: "6000",
            totalTokens: "2000",
            totalCost: "0.2",
          },
        ],
      ]
    );

    const result = await readArenaStats(redis as never, "test-job");

    expect(result).not.toBeNull();
    expect(result!.total).toBe(20);
    expect(Object.keys(result!.byProvider)).toHaveLength(2);
    expect(result!.byProvider["openai-gpt4"].total).toBe(10);
    expect(result!.byProvider["openai-gpt4"].avgLatencyMs).toBe(400);
    expect(result!.byProvider["anthropic-claude"].total).toBe(10);
    expect(result!.byProvider["anthropic-claude"].avgLatencyMs).toBe(600);
  });

  it("handles SCAN pagination", async () => {
    const redis = {
      hgetall: vi.fn(),
      scan: vi.fn(),
    };

    redis.hgetall.mockImplementation((key: string) => {
      if (key === "arena:job:test-job:stats") {
        return Promise.resolve({ total: "5", passed: "5", failed: "0", totalDurationMs: "1000", totalTokens: "100", totalCost: "0.01" });
      }
      if (key === "arena:job:test-job:stats:provider:p1") {
        return Promise.resolve({ total: "3", passed: "3", failed: "0", totalDurationMs: "600", totalTokens: "60", totalCost: "0.006" });
      }
      if (key === "arena:job:test-job:stats:provider:p2") {
        return Promise.resolve({ total: "2", passed: "2", failed: "0", totalDurationMs: "400", totalTokens: "40", totalCost: "0.004" });
      }
      return Promise.resolve({});
    });

    // First call returns cursor "42" and one key, second call returns "0" and another key
    redis.scan
      .mockResolvedValueOnce(["42", ["arena:job:test-job:stats:provider:p1"]])
      .mockResolvedValueOnce(["0", ["arena:job:test-job:stats:provider:p2"]]);

    const result = await readArenaStats(redis as never, "test-job");

    expect(result).not.toBeNull();
    expect(Object.keys(result!.byProvider)).toHaveLength(2);
    expect(result!.byProvider["p1"].total).toBe(3);
    expect(result!.byProvider["p2"].total).toBe(2);
    expect(redis.scan).toHaveBeenCalledTimes(2);
  });

  it("skips empty provider hashes", async () => {
    const redis = createMockRedis(
      { total: "5", passed: "5", failed: "0", totalDurationMs: "1000", totalTokens: "100", totalCost: "0.01" },
      [["arena:job:test-job:stats:provider:empty", {}]]
    );

    // Override hgetall for the provider key to return empty
    redis.hgetall.mockImplementation((key: string) => {
      if (key === "arena:job:test-job:stats") {
        return Promise.resolve({ total: "5", passed: "5", failed: "0", totalDurationMs: "1000", totalTokens: "100", totalCost: "0.01" });
      }
      return Promise.resolve({});
    });
    redis.scan.mockResolvedValue(["0", ["arena:job:test-job:stats:provider:empty"]]);

    const result = await readArenaStats(redis as never, "test-job");

    expect(result).not.toBeNull();
    expect(Object.keys(result!.byProvider)).toHaveLength(0);
  });
});

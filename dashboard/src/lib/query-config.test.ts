import { describe, it, expect } from "vitest";
import { DEFAULT_STALE_TIME, PROMETHEUS_FETCH_TIMEOUT_MS } from "./query-config";

describe("query-config", () => {
  it("DEFAULT_STALE_TIME is 30 seconds", () => {
    expect(DEFAULT_STALE_TIME).toBe(30_000);
  });

  it("PROMETHEUS_FETCH_TIMEOUT_MS is 30 seconds", () => {
    expect(PROMETHEUS_FETCH_TIMEOUT_MS).toBe(30_000);
  });
});

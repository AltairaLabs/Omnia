import { describe, it, expect } from "vitest";
import { generateMockProjection } from "./mock-projection";

const TIERS = ["institutional", "agent", "user", "user_for_agent"] as const;

describe("generateMockProjection", () => {
  it("is deterministic for a fixed seed", () => {
    const a = generateMockProjection({ seed: 7, count: 50 });
    const b = generateMockProjection({ seed: 7, count: 50 });
    expect(a.points).toEqual(b.points);
  });
  it("produces the requested number of points", () => {
    expect(generateMockProjection({ seed: 1, count: 120 }).points).toHaveLength(120);
  });
  it("includes all four tiers", () => {
    const tiers = new Set(generateMockProjection({ seed: 3, count: 400 }).points.map((p) => p.tier));
    for (const t of TIERS) expect(tiers.has(t)).toBe(true);
  });
  it("keeps coordinates within [-1, 1]", () => {
    for (const p of generateMockProjection({ seed: 9, count: 200 }).points) {
      expect(p.x).toBeGreaterThanOrEqual(-1);
      expect(p.x).toBeLessThanOrEqual(1);
      expect(p.y).toBeGreaterThanOrEqual(-1);
      expect(p.y).toBeLessThanOrEqual(1);
    }
  });
});

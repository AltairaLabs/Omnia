import { describe, it, expect } from "vitest";
import { roundedPath, midpoint } from "./workload-edge";

describe("roundedPath", () => {
  it("returns empty for fewer than two points", () => {
    expect(roundedPath([])).toBe("");
    expect(roundedPath([{ x: 0, y: 0 }])).toBe("");
  });

  it("draws a straight segment for two points", () => {
    expect(roundedPath([{ x: 0, y: 0 }, { x: 10, y: 0 }])).toBe("M 0,0 L 10,0");
  });

  it("rounds the corner at an interior bend with a quadratic curve", () => {
    const d = roundedPath([{ x: 0, y: 0 }, { x: 20, y: 0 }, { x: 20, y: 20 }], 6);
    expect(d).toMatch(/^M 0,0/);
    expect(d).toContain("Q 20,0"); // control point is the corner
    expect(d).toMatch(/L 20,20$/);
  });
});

describe("midpoint", () => {
  it("returns the centre waypoint", () => {
    expect(midpoint([{ x: 0, y: 0 }, { x: 5, y: 5 }, { x: 10, y: 10 }])).toEqual({ x: 5, y: 5 });
  });
  it("is safe for an empty route", () => {
    expect(midpoint([])).toEqual({ x: 0, y: 0 });
  });
});

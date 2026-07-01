import { describe, it, expect } from "vitest";
import {
  project,
  sizeFactor,
  pointRadius,
  overlaps,
  onScreen,
  lifeFraction,
  computeBubbles,
} from "./galaxy-layout";
import { fitTransform } from "./galaxy-math";
import type { GalaxyPoint } from "./types";

function pt(id: string, x: number, y: number, over: Partial<GalaxyPoint> = {}): GalaxyPoint {
  return { id, x, y, tier: "user", confidence: 0.5, ...over } as GalaxyPoint;
}

const view = { zoom: 1, panX: 0, panY: 0 };

describe("sizeFactor", () => {
  it("floors at 0.7 for tiny zoom", () => {
    expect(sizeFactor(0.001)).toBe(0.7);
  });
  it("is 1 at zoom 1 and grows with zoom", () => {
    expect(sizeFactor(1)).toBe(1);
    expect(sizeFactor(4)).toBeGreaterThan(sizeFactor(2));
  });
});

describe("pointRadius", () => {
  it("scales with confidence and the size factor", () => {
    const low = pointRadius(pt("a", 0, 0, { confidence: 0.1 }), 1);
    const high = pointRadius(pt("b", 0, 0, { confidence: 0.9 }), 1);
    expect(high).toBeGreaterThan(low);
    expect(pointRadius(pt("c", 0, 0, { confidence: 0.5 }), 2)).toBeCloseTo(
      pointRadius(pt("c", 0, 0, { confidence: 0.5 }), 1) * 2,
    );
  });
});

describe("overlaps", () => {
  it("detects overlapping boxes", () => {
    expect(overlaps({ x: 0, y: 0, w: 10, h: 10 }, { x: 5, y: 5, w: 10, h: 10 })).toBe(true);
  });
  it("returns false for disjoint boxes", () => {
    expect(overlaps({ x: 0, y: 0, w: 10, h: 10 }, { x: 20, y: 20, w: 5, h: 5 })).toBe(false);
  });
});

describe("onScreen", () => {
  it("accepts points within the padded viewport", () => {
    expect(onScreen({ x: 50, y: 50 }, 100, 100)).toBe(true);
  });
  it("rejects points well outside", () => {
    expect(onScreen({ x: -200, y: 50 }, 100, 100)).toBe(false);
  });
});

describe("lifeFraction", () => {
  const now = Date.parse("2026-07-01T00:00:00Z");
  it("is 0 without a TTL", () => {
    expect(lifeFraction(pt("a", 0, 0), now)).toBe(0);
  });
  it("is 0 when expiry precedes creation", () => {
    const p = pt("a", 0, 0, { observedAt: "2026-07-01T00:00:00Z", expiresAt: "2026-06-01T00:00:00Z" });
    expect(lifeFraction(p, now)).toBe(0);
  });
  it("is ~0.5 halfway through the lifetime", () => {
    const p = pt("a", 0, 0, { observedAt: "2026-06-01T00:00:00Z", expiresAt: "2026-08-01T00:00:00Z" });
    expect(lifeFraction(p, now)).toBeCloseTo(0.5, 1);
  });
  it("clamps to 1 past expiry", () => {
    const p = pt("a", 0, 0, { observedAt: "2026-01-01T00:00:00Z", expiresAt: "2026-02-01T00:00:00Z" });
    expect(lifeFraction(p, now)).toBe(1);
  });
});

describe("project", () => {
  it("applies zoom and pan on top of the world transform", () => {
    const pts = [pt("a", 0, 0), pt("b", 10, 10)];
    const fit = fitTransform(pts, 200, 200);
    const base = project(pts[0], fit, { zoom: 1, panX: 0, panY: 0 });
    const shifted = project(pts[0], fit, { zoom: 1, panX: 30, panY: 40 });
    expect(shifted.x).toBeCloseTo(base.x + 30);
    expect(shifted.y).toBeCloseTo(base.y + 40);
  });
});

describe("computeBubbles", () => {
  const pts = [pt("a", 0, 0), pt("b", 10, 10)];
  const size = { w: 200, h: 200 };
  const rect = { left: 0, top: 0 };

  it("returns nothing when no ids are open", () => {
    expect(computeBubbles(pts, new Set(), view, size, rect)).toEqual([]);
  });

  it("returns nothing without a canvas rect", () => {
    expect(computeBubbles(pts, new Set(["a"]), view, size, null)).toEqual([]);
  });

  it("anchors a bubble for an open, on-canvas point", () => {
    const out = computeBubbles(pts, new Set(["a"]), view, size, rect);
    expect(out).toHaveLength(1);
    expect(out[0].p.id).toBe("a");
    expect(["above", "below"]).toContain(out[0].placement);
    expect(Number.isFinite(out[0].left)).toBe(true);
  });

  it("skips ids that are not open", () => {
    const out = computeBubbles(pts, new Set(["b"]), view, size, rect);
    expect(out.every((b) => b.p.id === "b")).toBe(true);
  });
});

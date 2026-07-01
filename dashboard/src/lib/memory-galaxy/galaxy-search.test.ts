import { describe, it, expect } from "vitest";
import { matchesSearch, countMatches, matchFitView } from "./galaxy-search";
import { fitTransform, worldToScreen } from "./galaxy-math";
import type { GalaxyPoint } from "./types";

function pt(id: string, x: number, y: number, over: Partial<GalaxyPoint> = {}): GalaxyPoint {
  return { id, x, y, tier: "user", confidence: 0.5, ...over } as GalaxyPoint;
}

describe("matchesSearch", () => {
  it("matches everything on an empty query", () => {
    expect(matchesSearch(pt("a", 0, 0), "")).toBe(true);
  });
  it("matches on the title, case-insensitively", () => {
    expect(matchesSearch(pt("a", 0, 0, { title: "Billing Dispute" }), "billing")).toBe(true);
  });
  it("matches on the preview", () => {
    expect(matchesSearch(pt("a", 0, 0, { preview: "rate limit exceeded" }), "rate limit")).toBe(true);
  });
  it("does not match unrelated text", () => {
    expect(matchesSearch(pt("a", 0, 0, { title: "billing" }), "xyz")).toBe(false);
  });
  it("never matches a masked point (null title/preview)", () => {
    expect(matchesSearch(pt("a", 0, 0), "anything")).toBe(false);
  });
});

describe("countMatches", () => {
  const pts = [
    pt("a", 0, 0, { title: "billing dispute" }),
    pt("b", 0, 0, { title: "rate limit" }),
    pt("c", 0, 0, { preview: "billing question" }),
    pt("d", 0, 0), // masked
  ];
  it("returns 0 for an empty query", () => {
    expect(countMatches(pts, "")).toBe(0);
  });
  it("counts points whose title or preview match", () => {
    expect(countMatches(pts, "billing")).toBe(2);
  });
  it("returns 0 when nothing matches", () => {
    expect(countMatches(pts, "nope")).toBe(0);
  });
});

describe("matchFitView", () => {
  const pts = [
    pt("a", 0, 0, { title: "alpha" }),
    pt("b", 10, 10, { title: "beta" }),
    pt("c", -10, -10, { title: "alpha again" }),
  ];
  const fit = fitTransform(pts, 400, 400);

  it("returns null for an empty query", () => {
    expect(matchFitView(pts, "", fit, 400, 400, 0.5, 16)).toBeNull();
  });
  it("returns null when the canvas has no size", () => {
    expect(matchFitView(pts, "alpha", fit, 0, 0, 0.5, 16)).toBeNull();
  });
  it("returns null when nothing matches", () => {
    expect(matchFitView(pts, "zzz", fit, 400, 400, 0.5, 16)).toBeNull();
  });
  it("centres the matching set in the canvas", () => {
    // 'alpha' matches a (0,0) and c (-10,-10). Applying the returned view to both
    // matches, their midpoint should land at the canvas centre (200, 200).
    const v = matchFitView(pts, "alpha", fit, 400, 400, 0.5, 16)!;
    expect(v).not.toBeNull();
    const screen = (p: GalaxyPoint) => {
      const b = worldToScreen(p, fit);
      return { x: b.x * v.zoom + v.panX, y: b.y * v.zoom + v.panY };
    };
    const a = screen(pts[0]);
    const c = screen(pts[2]);
    expect((a.x + c.x) / 2).toBeCloseTo(200, 0);
    expect((a.y + c.y) / 2).toBeCloseTo(200, 0);
  });
  it("clamps zoom to maxZoom for a single tight match", () => {
    const v = matchFitView(pts, "beta", fit, 400, 400, 0.5, 16)!;
    expect(v.zoom).toBe(16);
  });
});

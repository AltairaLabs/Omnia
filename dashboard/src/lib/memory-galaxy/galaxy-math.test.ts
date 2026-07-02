import { describe, it, expect } from "vitest";
import {
  fitTransform, worldToScreen, hitTest, colorForPoint,
  isTierVisible, matchesFilters, legendCounts,
  parseHiddenTiers, serializeHiddenTiers, pointFacet, facetCounts,
} from "./galaxy-math";
import type { GalaxyPoint } from "./types";

const pt = (over: Partial<GalaxyPoint>): GalaxyPoint => ({
  id: "p", x: 0, y: 0, tier: "user", category: "memory:identity",
  confidence: 0.9, title: "Name is Dana", preview: "Dana likes tea", ...over,
});

describe("fitTransform", () => {
  it("centres a single point in the viewport", () => {
    const t = fitTransform([pt({ x: 0, y: 0 })], 200, 100);
    const s = worldToScreen(pt({ x: 0, y: 0 }), t);
    expect(s.x).toBeCloseTo(100, 0);
    expect(s.y).toBeCloseTo(50, 0);
  });
});
describe("hitTest", () => {
  it("returns the id of the nearest point within radius", () => {
    const pts = [pt({ id: "a", x: -0.5, y: 0 }), pt({ id: "b", x: 0.5, y: 0 })];
    const t = fitTransform(pts, 200, 100);
    const b = worldToScreen(pts[1], t);
    expect(hitTest(pts, b, t, 10)).toBe("b");
  });
  it("returns null when nothing is within radius", () => {
    const pts = [pt({ id: "a", x: 0, y: 0 })];
    const t = fitTransform(pts, 200, 100);
    expect(hitTest(pts, { x: -999, y: -999 }, t, 10)).toBeNull();
  });
});
describe("colorForPoint", () => {
  it("uses tier color when colorBy=tier", () => {
    expect(colorForPoint(pt({ tier: "institutional" }), "tier")).toBe("hsl(217, 91%, 60%)");
  });
  it("uses category color when colorBy=category", () => {
    expect(colorForPoint(pt({ category: "memory:identity" }), "category")).toBe("#3B82F6");
  });
  it("falls back to unknown category color", () => {
    expect(colorForPoint(pt({ category: undefined }), "category")).toBe("#6B7280");
  });
});
describe("visibility + filters", () => {
  it("isTierVisible hides tiers in the hidden set", () => {
    expect(isTierVisible(pt({ tier: "agent" }), new Set(["agent"]))).toBe(false);
    expect(isTierVisible(pt({ tier: "user" }), new Set(["agent"]))).toBe(true);
  });
  it("matchesFilters dims on category mismatch", () => {
    expect(matchesFilters(pt({ category: "memory:health" }), { category: "memory:identity", search: "" })).toBe(false);
  });
  it("matchesFilters matches search against title and preview", () => {
    expect(matchesFilters(pt({ title: "Name is Dana" }), { category: "all", search: "dana" })).toBe(true);
    expect(matchesFilters(pt({ title: "x", preview: "y" }), { category: "all", search: "zzz" })).toBe(false);
  });
});
describe("legendCounts", () => {
  it("always returns all four tiers, zero-filled", () => {
    const counts = legendCounts([pt({ tier: "user" }), pt({ tier: "user" })]);
    expect(counts).toEqual({ institutional: 0, agent: 0, user: 2, user_for_agent: 0 });
  });
});
describe("hidden-tier serialization", () => {
  it("round-trips a set through a csv string", () => {
    const s = serializeHiddenTiers(new Set(["agent", "institutional"]));
    expect(parseHiddenTiers(s)).toEqual(new Set(["agent", "institutional"]));
  });
  it("parses empty string to an empty set", () => {
    expect(parseHiddenTiers("")).toEqual(new Set());
  });
});
describe("pointFacet / facetCounts", () => {
  it("returns the tier or category for the active dimension", () => {
    expect(pointFacet(pt({ tier: "agent" }), "tier")).toBe("agent");
    expect(pointFacet(pt({ category: "memory:health" }), "category")).toBe("memory:health");
    expect(pointFacet(pt({ category: undefined }), "category")).toBe("unknown");
  });
  it("counts points per facet for the active dimension", () => {
    const pts = [
      pt({ tier: "user", category: "memory:identity" }),
      pt({ tier: "user", category: "memory:health" }),
      pt({ tier: "agent", category: "memory:identity" }),
    ];
    expect(facetCounts(pts, "tier")).toEqual({ user: 2, agent: 1 });
    expect(facetCounts(pts, "category")).toEqual({ "memory:identity": 2, "memory:health": 1 });
  });
});

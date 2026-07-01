import { describe, it, expect } from "vitest";
import { sortStack, clampIndex, removeAt } from "./stack";
import type { GalaxyPoint } from "./types";

function pt(id: string, category?: string): GalaxyPoint {
  return { id, x: 0, y: 0, tier: "user", category, confidence: 0.5 } as GalaxyPoint;
}

describe("sortStack", () => {
  it("leads with the point nearest the click", () => {
    const pts = [pt("far", "b"), pt("near", "a"), pt("mid", "c")];
    const dist = (p: GalaxyPoint) => ({ far: 100, near: 1, mid: 25 })[p.id]!;
    const out = sortStack(pts, dist);
    expect(out[0].id).toBe("near");
  });

  it("groups points by category (category of nearest member first)", () => {
    // Two categories interleaved; nearest point is in category 'a'.
    const pts = [pt("a1", "a"), pt("b1", "b"), pt("a2", "a"), pt("b2", "b")];
    const dist = (p: GalaxyPoint) => ({ a1: 1, a2: 4, b1: 9, b2: 16 })[p.id]!;
    const out = sortStack(pts, dist).map((p) => p.id);
    // Category 'a' (nearest member dist 1) leads, then 'b'; nearest-first within.
    expect(out).toEqual(["a1", "a2", "b1", "b2"]);
  });

  it("orders nearest-first within a single category", () => {
    const pts = [pt("c", "x"), pt("a", "x"), pt("b", "x")];
    const dist = (p: GalaxyPoint) => ({ a: 1, b: 2, c: 3 })[p.id]!;
    expect(sortStack(pts, dist).map((p) => p.id)).toEqual(["a", "b", "c"]);
  });

  it("treats missing category as its own group without throwing", () => {
    const pts = [pt("u1"), pt("k1", "k")];
    const dist = (p: GalaxyPoint) => ({ u1: 5, k1: 1 })[p.id]!;
    // 'k' (dist 1) ranks before the empty category (dist 5).
    expect(sortStack(pts, dist).map((p) => p.id)).toEqual(["k1", "u1"]);
  });

  it("does not mutate the input array", () => {
    const pts = [pt("b", "b"), pt("a", "a")];
    const before = pts.map((p) => p.id);
    sortStack(pts, () => 0);
    expect(pts.map((p) => p.id)).toEqual(before);
  });

  it("returns an empty array for no points", () => {
    expect(sortStack([], () => 0)).toEqual([]);
  });
});

describe("clampIndex", () => {
  it("returns 0 for an empty stack", () => {
    expect(clampIndex(3, 0)).toBe(0);
    expect(clampIndex(-1, 0)).toBe(0);
  });
  it("clamps below zero to zero", () => {
    expect(clampIndex(-5, 4)).toBe(0);
  });
  it("clamps past the end to the last index", () => {
    expect(clampIndex(9, 4)).toBe(3);
  });
  it("passes an in-range index through", () => {
    expect(clampIndex(2, 4)).toBe(2);
  });
});

describe("removeAt", () => {
  it("removes the middle item and keeps the index on the next one", () => {
    const stack = [pt("0"), pt("1"), pt("2"), pt("3")];
    const { stack: next, index } = removeAt(stack, 1);
    expect(next.map((p) => p.id)).toEqual(["0", "2", "3"]);
    expect(index).toBe(1); // now points at "2"
  });

  it("removing the last item clamps the index back", () => {
    const stack = [pt("0"), pt("1"), pt("2")];
    const { stack: next, index } = removeAt(stack, 2);
    expect(next.map((p) => p.id)).toEqual(["0", "1"]);
    expect(index).toBe(1);
  });

  it("removing the only item yields an empty stack and index 0", () => {
    const { stack: next, index } = removeAt([pt("only")], 0);
    expect(next).toEqual([]);
    expect(index).toBe(0);
  });
});

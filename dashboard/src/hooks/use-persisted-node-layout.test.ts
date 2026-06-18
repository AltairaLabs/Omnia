import { describe, it, expect, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { Node } from "@xyflow/react";
import { usePersistedNodeLayout, loadExpanded, saveExpanded } from "./use-persisted-node-layout";

beforeEach(() => window.localStorage.clear());

const node = (id: string, x: number, y: number): Node =>
  ({ id, position: { x, y }, data: {} }) as Node;

describe("usePersistedNodeLayout", () => {
  it("persists a dragged node and applies it back, leaving others on their layout", () => {
    const { result } = renderHook(() => usePersistedNodeLayout("k"));
    result.current.onNodeDragStop({}, node("a", 5, 9));

    const applied = result.current.applyLayout([node("a", 0, 0), node("b", 1, 1)]);

    expect(applied[0].position).toEqual({ x: 5, y: 9 });
    expect(applied[1].position).toEqual({ x: 1, y: 1 });
  });

  it("reset clears the saved layout so elk positions win again", () => {
    const { result } = renderHook(() => usePersistedNodeLayout("k"));
    result.current.onNodeDragStop({}, node("a", 5, 9));
    result.current.reset();
    expect(result.current.applyLayout([node("a", 0, 0)])[0].position).toEqual({ x: 0, y: 0 });
  });

  it("keeps separate layouts per key", () => {
    const { result: a } = renderHook(() => usePersistedNodeLayout("packA"));
    const { result: b } = renderHook(() => usePersistedNodeLayout("packB"));
    a.current.onNodeDragStop({}, node("n", 7, 7));
    expect(b.current.applyLayout([node("n", 0, 0)])[0].position).toEqual({ x: 0, y: 0 });
  });
});

describe("loadExpanded / saveExpanded", () => {
  it("round-trips an expanded set under a :expanded sub-key", () => {
    saveExpanded("k", new Set(["main", "other"]));
    expect(loadExpanded("k")).toEqual(new Set(["main", "other"]));
    // stored under the namespaced key, not the bare layout key
    expect(window.localStorage.getItem("k:expanded")).toBe(JSON.stringify(["main", "other"]));
  });

  it("returns an empty set for a missing key or an empty storage key", () => {
    expect(loadExpanded("missing")).toEqual(new Set());
    expect(loadExpanded("")).toEqual(new Set());
  });

  it("returns an empty set when the stored value is malformed", () => {
    window.localStorage.setItem("bad:expanded", "{not json");
    expect(loadExpanded("bad")).toEqual(new Set());
  });

  it("is a no-op when saving with an empty storage key", () => {
    saveExpanded("", new Set(["x"]));
    expect(window.localStorage.getItem(":expanded")).toBeNull();
  });
});

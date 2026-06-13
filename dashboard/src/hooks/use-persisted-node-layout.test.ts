import { describe, it, expect, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { Node } from "@xyflow/react";
import { usePersistedNodeLayout } from "./use-persisted-node-layout";

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

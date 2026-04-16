import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { useAdjacentSessions } from "./use-adjacent-sessions";

vi.mock("./use-sessions", () => ({
  useSessions: vi.fn(),
}));

const useSessionsMock = vi.mocked(await import("./use-sessions")).useSessions as unknown as ReturnType<
  typeof vi.fn
>;

function setSessionList(ids: string[]) {
  useSessionsMock.mockReturnValue({
    data: {
      sessions: ids.map((id) => ({ id })),
      total: ids.length,
      hasMore: false,
    },
  } as never);
}

describe("useAdjacentSessions", () => {
  it("returns null ids when the list is empty", () => {
    setSessionList([]);
    const { result } = renderHook(() => useAdjacentSessions("anything"));
    expect(result.current).toEqual({ prevId: null, nextId: null, position: null, total: 0 });
  });

  it("returns null ids when currentId is not in the list", () => {
    setSessionList(["a", "b", "c"]);
    const { result } = renderHook(() => useAdjacentSessions("zzz"));
    expect(result.current).toEqual({ prevId: null, nextId: null, position: null, total: 3 });
  });

  it("returns only nextId for the first session", () => {
    setSessionList(["a", "b", "c"]);
    const { result } = renderHook(() => useAdjacentSessions("a"));
    expect(result.current.prevId).toBe(null);
    expect(result.current.nextId).toBe("b");
    expect(result.current.position).toBe(1);
    expect(result.current.total).toBe(3);
  });

  it("returns both prev and next for a middle session", () => {
    setSessionList(["a", "b", "c"]);
    const { result } = renderHook(() => useAdjacentSessions("b"));
    expect(result.current).toEqual({ prevId: "a", nextId: "c", position: 2, total: 3 });
  });

  it("returns only prevId for the last session", () => {
    setSessionList(["a", "b", "c"]);
    const { result } = renderHook(() => useAdjacentSessions("c"));
    expect(result.current.prevId).toBe("b");
    expect(result.current.nextId).toBe(null);
  });
});

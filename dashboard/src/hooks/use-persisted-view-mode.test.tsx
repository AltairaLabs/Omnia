import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { usePersistedViewMode } from "./use-persisted-view-mode";

type Mode = "cards" | "table";

beforeEach(() => window.localStorage.clear());

describe("usePersistedViewMode", () => {
  it("returns the default when nothing is stored", () => {
    const { result } = renderHook(() => usePersistedViewMode<Mode>("k", "cards"));
    expect(result.current[0]).toBe("cards");
  });

  it("reads a previously stored value", () => {
    window.localStorage.setItem("k", "table");
    const { result } = renderHook(() => usePersistedViewMode<Mode>("k", "cards"));
    expect(result.current[0]).toBe("table");
  });

  it("writes the new value on change and persists it", () => {
    const { result } = renderHook(() => usePersistedViewMode<Mode>("k", "cards"));
    act(() => result.current[1]("table"));
    expect(result.current[0]).toBe("table");
    expect(window.localStorage.getItem("k")).toBe("table");
  });

  it("isolates values by key", () => {
    const a = renderHook(() => usePersistedViewMode<Mode>("a", "cards"));
    act(() => a.result.current[1]("table"));
    const b = renderHook(() => usePersistedViewMode<Mode>("b", "cards"));
    expect(b.result.current[0]).toBe("cards");
  });
});

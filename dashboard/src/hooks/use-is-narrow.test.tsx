import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useIsNarrow } from "./use-is-narrow";

function mockMatchMedia(initial: boolean) {
  let handler: (() => void) | undefined;
  const mql = {
    matches: initial,
    media: "",
    onchange: null,
    addEventListener: (_: string, h: () => void) => {
      handler = h;
    },
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  };
  window.matchMedia = vi
    .fn()
    .mockReturnValue(mql) as unknown as typeof window.matchMedia;
  return {
    fire: (matches: boolean) => {
      mql.matches = matches;
      handler?.();
    },
  };
}

beforeEach(() => vi.restoreAllMocks());

describe("useIsNarrow", () => {
  it("returns true when the media query matches on mount", () => {
    mockMatchMedia(true);
    const { result } = renderHook(() => useIsNarrow());
    expect(result.current).toBe(true);
  });

  it("returns false when the media query does not match", () => {
    mockMatchMedia(false);
    const { result } = renderHook(() => useIsNarrow());
    expect(result.current).toBe(false);
  });

  it("updates when the media query changes", () => {
    const { fire } = mockMatchMedia(false);
    const { result } = renderHook(() => useIsNarrow());
    expect(result.current).toBe(false);
    act(() => fire(true));
    expect(result.current).toBe(true);
  });
});

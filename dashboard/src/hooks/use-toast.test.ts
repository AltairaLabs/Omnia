/**
 * Tests for useToast hook.
 */

import { describe, it, expect } from "vitest";
import { renderHook } from "@testing-library/react";
import { useToast } from "./use-toast";

describe("useToast", () => {
  it("returns toast function", () => {
    const { result } = renderHook(() => useToast());

    expect(result.current.toast).toBeDefined();
    expect(typeof result.current.toast).toBe("function");
  });

  it("toast function can be called without error", () => {
    const { result } = renderHook(() => useToast());

    // Should not throw
    expect(() => {
      result.current.toast({ title: "Test" });
    }).not.toThrow();

    expect(() => {
      result.current.toast({ title: "Error", description: "Something went wrong", variant: "destructive" });
    }).not.toThrow();
  });

  it("returns stable toast function reference", () => {
    const { result, rerender } = renderHook(() => useToast());
    const firstToast = result.current.toast;

    rerender();

    expect(result.current.toast).toBe(firstToast);
  });
});

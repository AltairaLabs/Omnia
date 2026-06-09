/**
 * Tests for useToast hook.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useToast, toast, dismissToast } from "./use-toast";

describe("useToast", () => {
  beforeEach(() => {
    // Clear any toasts left from a prior test (module-level store).
    const { result } = renderHook(() => useToast());
    act(() => {
      for (const t of result.current.toasts) dismissToast(t.id);
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("returns a callable, stable toast function", () => {
    const { result, rerender } = renderHook(() => useToast());
    expect(typeof result.current.toast).toBe("function");
    const first = result.current.toast;
    rerender();
    expect(result.current.toast).toBe(first);
  });

  it("exposes a toast added via toast()", () => {
    const { result } = renderHook(() => useToast());
    act(() => {
      result.current.toast({ title: "Saved", description: "All good" });
    });
    expect(result.current.toasts).toHaveLength(1);
    expect(result.current.toasts[0]).toMatchObject({
      title: "Saved",
      description: "All good",
    });
  });

  it("dismiss removes a toast", () => {
    const { result } = renderHook(() => useToast());
    let id = "";
    act(() => {
      id = result.current.toast({ title: "X" });
    });
    expect(result.current.toasts).toHaveLength(1);
    act(() => {
      result.current.dismiss(id);
    });
    expect(result.current.toasts).toHaveLength(0);
  });

  it("auto-dismisses after the duration", () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useToast());
    act(() => {
      result.current.toast({ title: "ephemeral", duration: 3000 });
    });
    expect(result.current.toasts).toHaveLength(1);
    act(() => {
      vi.advanceTimersByTime(3000);
    });
    expect(result.current.toasts).toHaveLength(0);
  });

  it("shares state across hook instances (module store)", () => {
    const a = renderHook(() => useToast());
    const b = renderHook(() => useToast());
    act(() => {
      toast({ title: "broadcast" });
    });
    expect(a.result.current.toasts).toHaveLength(1);
    expect(b.result.current.toasts).toHaveLength(1);
  });
});

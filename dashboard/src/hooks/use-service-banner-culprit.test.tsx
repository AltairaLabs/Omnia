/**
 * Tests for useServiceBannerCulprit — the shared tri-state used by the
 * sessions/memory pages to proactively surface the ServiceUnreadyBanner
 * (while loading OR errored, not just once an error has landed) and to
 * reset that tri-state at the start of each new fetch cycle.
 */

import { describe, it, expect } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useServiceBannerCulprit } from "./use-service-banner-culprit";

describe("useServiceBannerCulprit", () => {
  it("does not show the banner when idle (not loading, no error)", () => {
    const { result } = renderHook(() => useServiceBannerCulprit("ws-1", undefined, false));

    expect(result.current.showBanner).toBe(false);
    expect(result.current.bannerCulprit).toBeUndefined();
  });

  it("shows the banner while loading, even with no error yet", () => {
    const { result } = renderHook(() => useServiceBannerCulprit("ws-1", undefined, true));

    expect(result.current.showBanner).toBe(true);
  });

  it("shows the banner once an error has landed", () => {
    const error = new Error("boom");
    const { result } = renderHook(() => useServiceBannerCulprit("ws-1", error, false));

    expect(result.current.showBanner).toBe(true);
  });

  it("resolves bannerCulprit to false immediately when there is no workspace", () => {
    const error = new Error("boom");
    const { result } = renderHook(() => useServiceBannerCulprit(undefined, error, false));

    expect(result.current.bannerCulprit).toBe(false);
  });

  it("lets the caller report a culprit via setBannerCulprit", () => {
    const error = new Error("boom");
    const { result } = renderHook(() => useServiceBannerCulprit("ws-1", error, false));

    act(() => {
      result.current.setBannerCulprit(true);
    });

    expect(result.current.bannerCulprit).toBe(true);
  });

  it("resets bannerCulprit to pending (undefined) when the error identity changes", () => {
    const { result, rerender } = renderHook(
      ({ error }: { error: unknown }) => useServiceBannerCulprit("ws-1", error, false),
      { initialProps: { error: new Error("first") } }
    );

    act(() => {
      result.current.setBannerCulprit(true);
    });
    expect(result.current.bannerCulprit).toBe(true);

    rerender({ error: new Error("second") });

    expect(result.current.bannerCulprit).toBeUndefined();
  });

  it("resets bannerCulprit to pending when the workspace changes", () => {
    const error = new Error("boom");
    const { result, rerender } = renderHook(
      ({ workspaceName }: { workspaceName: string }) =>
        useServiceBannerCulprit(workspaceName, error, false),
      { initialProps: { workspaceName: "ws-1" } }
    );

    act(() => {
      result.current.setBannerCulprit(true);
    });
    expect(result.current.bannerCulprit).toBe(true);

    rerender({ workspaceName: "ws-2" });

    expect(result.current.bannerCulprit).toBeUndefined();
  });

  it("resets bannerCulprit to pending when a new loading cycle starts", () => {
    const { result, rerender } = renderHook(
      ({ isLoading }: { isLoading: boolean }) => useServiceBannerCulprit("ws-1", undefined, isLoading),
      { initialProps: { isLoading: false } }
    );

    act(() => {
      result.current.setBannerCulprit(true);
    });
    expect(result.current.bannerCulprit).toBe(true);

    // A fresh fetch starts (isLoading flips false -> true) with the same
    // (falsy) error identity — the stale culprit from the last cycle must
    // not leak into the new one.
    rerender({ isLoading: true });

    expect(result.current.bannerCulprit).toBeUndefined();
  });

  it("does not reset bannerCulprit when loading ends without an error (success)", () => {
    const { result, rerender } = renderHook(
      ({ isLoading }: { isLoading: boolean }) => useServiceBannerCulprit("ws-1", undefined, isLoading),
      { initialProps: { isLoading: true } }
    );

    act(() => {
      result.current.setBannerCulprit(false);
    });
    expect(result.current.bannerCulprit).toBe(false);

    // The fetch that started this loading cycle succeeded — isLoading ends
    // with no error. bannerCulprit should not be disturbed, and a
    // subsequent loading cycle should still be treated as "new" (covering
    // the internal wasLoading bookkeeping on the non-reset branch).
    rerender({ isLoading: false });
    expect(result.current.bannerCulprit).toBe(false);

    rerender({ isLoading: true });
    expect(result.current.bannerCulprit).toBeUndefined();
  });

  it("does not reset while isLoading stays continuously true", () => {
    const { result, rerender } = renderHook(
      ({ isLoading }: { isLoading: boolean }) => useServiceBannerCulprit("ws-1", undefined, isLoading),
      { initialProps: { isLoading: true } }
    );

    act(() => {
      result.current.setBannerCulprit(true);
    });
    expect(result.current.bannerCulprit).toBe(true);

    rerender({ isLoading: true });

    expect(result.current.bannerCulprit).toBe(true);
  });
});

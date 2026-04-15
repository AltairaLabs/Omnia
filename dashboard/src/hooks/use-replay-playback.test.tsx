import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useReplayPlayback } from "./use-replay-playback";

// jsdom does not implement requestAnimationFrame the way production does.
// Bind it to setTimeout so vi.useFakeTimers() can drive it deterministically.
let rafId = 0;
const originalRaf = global.requestAnimationFrame;
const originalCancelRaf = global.cancelAnimationFrame;

beforeEach(() => {
  rafId = 0;
  global.requestAnimationFrame = (cb: FrameRequestCallback) => {
    rafId++;
    const id = rafId;
    setTimeout(() => cb(performance.now()), 16);
    return id;
  };
  global.cancelAnimationFrame = (id: number) => clearTimeout(id as unknown as NodeJS.Timeout);
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  global.requestAnimationFrame = originalRaf;
  global.cancelAnimationFrame = originalCancelRaf;
});

describe("useReplayPlayback", () => {
  it("starts paused at time 0", () => {
    const { result } = renderHook(() => useReplayPlayback({ durationMs: 1000 }));
    expect(result.current.playing).toBe(false);
    expect(result.current.currentTimeMs).toBe(0);
    expect(result.current.speed).toBe(1);
  });

  it("advances currentTimeMs while playing", () => {
    const { result } = renderHook(() => useReplayPlayback({ durationMs: 10_000 }));
    act(() => result.current.play());
    act(() => {
      vi.advanceTimersByTime(500);
    });
    expect(result.current.currentTimeMs).toBeGreaterThanOrEqual(400);
    expect(result.current.currentTimeMs).toBeLessThanOrEqual(600);
  });

  it("auto-pauses when reaching end of session", () => {
    const { result } = renderHook(() => useReplayPlayback({ durationMs: 200 }));
    act(() => result.current.play());
    act(() => {
      vi.advanceTimersByTime(500);
    });
    expect(result.current.playing).toBe(false);
    expect(result.current.currentTimeMs).toBe(200);
  });

  it("applies speed multiplier", () => {
    const { result } = renderHook(() => useReplayPlayback({ durationMs: 10_000 }));
    act(() => result.current.setSpeed(4));
    act(() => result.current.play());
    act(() => {
      vi.advanceTimersByTime(500);
    });
    expect(result.current.currentTimeMs).toBeGreaterThanOrEqual(1800);
    expect(result.current.currentTimeMs).toBeLessThanOrEqual(2200);
  });

  it("seek() jumps directly, clamped to [0, duration]", () => {
    const { result } = renderHook(() => useReplayPlayback({ durationMs: 1000 }));
    act(() => result.current.seek(500));
    expect(result.current.currentTimeMs).toBe(500);
    act(() => result.current.seek(99999));
    expect(result.current.currentTimeMs).toBe(1000);
    act(() => result.current.seek(-10));
    expect(result.current.currentTimeMs).toBe(0);
  });

  it("pause() stops advancement", () => {
    const { result } = renderHook(() => useReplayPlayback({ durationMs: 10_000 }));
    act(() => result.current.play());
    act(() => vi.advanceTimersByTime(300));
    act(() => result.current.pause());
    const frozen = result.current.currentTimeMs;
    act(() => vi.advanceTimersByTime(300));
    expect(result.current.currentTimeMs).toBe(frozen);
  });

  it("play() from end restarts from 0", () => {
    const { result } = renderHook(() => useReplayPlayback({ durationMs: 200 }));
    act(() => result.current.seek(200));
    act(() => result.current.play());
    // First frame of play starts from 0, not from 200
    expect(result.current.currentTimeMs).toBeLessThan(200);
  });
});

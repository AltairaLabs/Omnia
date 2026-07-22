import { describe, it, expect, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useMicCapture } from "./use-mic-capture";

describe("useMicCapture", () => {
  it("requests the mic with echo cancellation and an AudioContext at the configured rate", async () => {
    const onFrame = vi.fn();
    const { result } = renderHook(() => useMicCapture({ sampleRate: 24000, channels: 1, onFrame }));
    await act(async () => { await result.current.start(); });
    expect(navigator.mediaDevices.getUserMedia).toHaveBeenCalledWith(
      expect.objectContaining({ audio: expect.objectContaining({ echoCancellation: true }) }),
    );
    expect(AudioContext).toHaveBeenCalledWith(expect.objectContaining({ sampleRate: 24000 }));
  });

  it("reconfigure() restarts capture at the new sample rate when running", async () => {
    const onFrame = vi.fn();
    const gum = navigator.mediaDevices.getUserMedia as ReturnType<typeof vi.fn>;
    const { result } = renderHook(() => useMicCapture({ sampleRate: 16000, channels: 1, onFrame }));
    await act(async () => { await result.current.start(); });
    expect(AudioContext).toHaveBeenLastCalledWith(expect.objectContaining({ sampleRate: 16000 }));
    const afterStart = gum.mock.calls.length;

    await act(async () => { await result.current.reconfigure(24000, 1); });
    expect(AudioContext).toHaveBeenLastCalledWith(expect.objectContaining({ sampleRate: 24000 }));
    expect(gum.mock.calls.length).toBe(afterStart + 1);
  });

  it("reconfigure() before start only updates the pending format (no capture)", async () => {
    const onFrame = vi.fn();
    const gum = navigator.mediaDevices.getUserMedia as ReturnType<typeof vi.fn>;
    const { result } = renderHook(() => useMicCapture({ sampleRate: 16000, channels: 1, onFrame }));
    const before = gum.mock.calls.length;
    await act(async () => { await result.current.reconfigure(24000, 2); });
    expect(gum.mock.calls.length).toBe(before);
    expect(result.current.format.current).toEqual({ sampleRate: 24000, channels: 2 });
  });

  it("reconfigure() is a no-op when the format is unchanged", async () => {
    const onFrame = vi.fn();
    const { result } = renderHook(() => useMicCapture({ sampleRate: 24000, channels: 1, onFrame }));
    await act(async () => { await result.current.start(); });
    const callsAfterStart = (navigator.mediaDevices.getUserMedia as ReturnType<typeof vi.fn>).mock.calls.length;

    await act(async () => { await result.current.reconfigure(24000, 1); });
    expect((navigator.mediaDevices.getUserMedia as ReturnType<typeof vi.fn>).mock.calls.length).toBe(callsAfterStart);
  });

  it("stop() releases the mic tracks", async () => {
    const stopTrack = vi.fn();
    (navigator.mediaDevices.getUserMedia as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ getTracks: () => [{ stop: stopTrack }] });
    const { result } = renderHook(() => useMicCapture({ sampleRate: 24000, channels: 1, onFrame: vi.fn() }));
    await act(async () => { await result.current.start(); });
    act(() => result.current.stop());
    expect(stopTrack).toHaveBeenCalled();
  });
});

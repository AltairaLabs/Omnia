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

  it("stop() releases the mic tracks", async () => {
    const stopTrack = vi.fn();
    (navigator.mediaDevices.getUserMedia as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ getTracks: () => [{ stop: stopTrack }] });
    const { result } = renderHook(() => useMicCapture({ sampleRate: 24000, channels: 1, onFrame: vi.fn() }));
    await act(async () => { await result.current.start(); });
    act(() => result.current.stop());
    expect(stopTrack).toHaveBeenCalled();
  });
});

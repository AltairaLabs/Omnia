import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useStreamingPlayback } from "./use-streaming-playback";

describe("useStreamingPlayback", () => {
  beforeEach(() => {
    // Reset the AudioContext spy between tests so each hook gets a fresh context
    vi.mocked(AudioContext).mockClear();
  });

  it("schedules a buffer source per enqueued chunk", () => {
    const { result } = renderHook(() => useStreamingPlayback({ sampleRate: 24000 }));

    act(() => {
      result.current.enqueue(new Int16Array([0, 1000, -1000, 0]).buffer);
    });
    act(() => {
      result.current.enqueue(new Int16Array([5, 6]).buffer);
    });

    // AudioContext should have been constructed once
    expect(AudioContext).toHaveBeenCalledTimes(1);

    // Retrieve the fake context instance
    const fakeCtx = vi.mocked(AudioContext).mock.results[0].value as {
      createBufferSource: ReturnType<typeof vi.fn>;
      createBuffer: ReturnType<typeof vi.fn>;
    };

    // One source created and started per enqueued chunk
    expect(fakeCtx.createBufferSource).toHaveBeenCalledTimes(2);
    expect(fakeCtx.createBuffer).toHaveBeenCalledTimes(2);

    const source0 = fakeCtx.createBufferSource.mock.results[0].value as { start: ReturnType<typeof vi.fn> };
    const source1 = fakeCtx.createBufferSource.mock.results[1].value as { start: ReturnType<typeof vi.fn> };
    expect(source0.start).toHaveBeenCalledTimes(1);
    expect(source1.start).toHaveBeenCalledTimes(1);
  });

  it("flush() stops all scheduled sources and clears them", () => {
    const { result } = renderHook(() => useStreamingPlayback({ sampleRate: 24000 }));

    act(() => {
      result.current.enqueue(new Int16Array([1, 2]).buffer);
    });
    act(() => {
      result.current.enqueue(new Int16Array([3, 4]).buffer);
    });

    const fakeCtx = vi.mocked(AudioContext).mock.results[0].value as {
      createBufferSource: ReturnType<typeof vi.fn>;
    };

    const source0 = fakeCtx.createBufferSource.mock.results[0].value as { stop: ReturnType<typeof vi.fn> };
    const source1 = fakeCtx.createBufferSource.mock.results[1].value as { stop: ReturnType<typeof vi.fn> };

    act(() => {
      result.current.flush();
    });

    // Both sources must have been stopped
    expect(source0.stop).toHaveBeenCalledTimes(1);
    expect(source1.stop).toHaveBeenCalledTimes(1);
  });

  it("stop() closes the AudioContext", () => {
    const { result } = renderHook(() => useStreamingPlayback({ sampleRate: 24000 }));

    act(() => {
      result.current.enqueue(new Int16Array([7, 8]).buffer);
    });

    const fakeCtx = vi.mocked(AudioContext).mock.results[0].value as {
      close: ReturnType<typeof vi.fn>;
    };

    act(() => {
      result.current.stop();
    });

    expect(fakeCtx.close).toHaveBeenCalledTimes(1);
  });
});

import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { MessageTypeInterrupt } from "../../../types/generated/websocket";

const startMic = vi.fn().mockResolvedValue(undefined);
const stopMic = vi.fn();
vi.mock("./use-mic-capture", () => ({
  useMicCapture: () => ({ start: startMic, stop: stopMic, level: 0 }),
}));

const flush = vi.fn();
const enqueue = vi.fn();
const stopPlay = vi.fn();
vi.mock("./use-streaming-playback", () => ({
  useStreamingPlayback: () => ({ enqueue, flush, stop: stopPlay }),
}));

const conn = {
  startAudioSession: vi.fn(),
  sendBinary: vi.fn(),
  disconnect: vi.fn(),
  onBinaryMedia: vi.fn().mockReturnValue(() => {}),
  onMessage: vi.fn().mockReturnValue(() => {}),
  onStatusChange: vi.fn().mockReturnValue(() => {}),
};
vi.mock("../../../lib/data/live-service", () => ({
  LiveAgentConnection: vi.fn(function () { return conn; } as any),
}));

import { useVoiceSession } from "./use-voice-session";

describe("useVoiceSession", () => {
  beforeEach(() => vi.clearAllMocks());

  it("call() goes requesting-mic then connecting and starts the audio session", async () => {
    const { result } = renderHook(() =>
      useVoiceSession({ namespace: "default", agentName: "v", sampleRate: 24000, channels: 1 }),
    );
    expect(result.current.state).toBe("idle");
    await act(async () => {
      result.current.call();
    });
    expect(startMic).toHaveBeenCalled();
    expect(conn.startAudioSession).toHaveBeenCalled();
  });

  it("hangup() stops mic+playback, disconnects, returns to idle", async () => {
    const { result } = renderHook(() =>
      useVoiceSession({ namespace: "default", agentName: "v", sampleRate: 24000, channels: 1 }),
    );
    await act(async () => {
      result.current.call();
    });
    act(() => {
      result.current.hangup();
    });
    expect(stopMic).toHaveBeenCalled();
    expect(conn.disconnect).toHaveBeenCalled();
    await waitFor(() => expect(result.current.state).toBe("idle"));
  });

  it("call() sets error state and stops the mic when mic.start() rejects", async () => {
    startMic.mockRejectedValueOnce(new Error("mic denied"));
    const { result } = renderHook(() =>
      useVoiceSession({ namespace: "default", agentName: "v", sampleRate: 24000, channels: 1 }),
    );
    await act(async () => {
      result.current.call();
    });
    await waitFor(() => expect(result.current.state).toBe("error"));
    expect(stopMic).toHaveBeenCalled();
    expect(conn.startAudioSession).not.toHaveBeenCalled();
  });

  it("an interrupt server message flushes playback and is not forwarded to onServerMessage", async () => {
    const onServerMessage = vi.fn();
    renderHook(() =>
      useVoiceSession({ namespace: "default", agentName: "v", sampleRate: 24000, channels: 1, onServerMessage }),
    );
    // Pull the onMessage handler registered by the hook
    const onMessageHandler = conn.onMessage.mock.calls[0][0] as (m: { type: string }) => void;
    act(() => {
      onMessageHandler({ type: MessageTypeInterrupt });
    });
    expect(flush).toHaveBeenCalled();
    expect(onServerMessage).not.toHaveBeenCalled();
  });
});

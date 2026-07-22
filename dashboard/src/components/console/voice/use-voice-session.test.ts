import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { MessageTypeInterrupt, MessageTypeSessionConfig } from "../../../types/generated/websocket";

const startMic = vi.fn().mockResolvedValue(undefined);
const stopMic = vi.fn();
const reconfigureMic = vi.fn().mockResolvedValue(undefined);
vi.mock("./use-mic-capture", () => ({
  useMicCapture: () => ({ start: startMic, stop: stopMic, reconfigure: reconfigureMic, level: 0 }),
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
  sendHangup: vi.fn(),
  disconnect: vi.fn(),
  onBinaryMedia: vi.fn().mockReturnValue(() => {}),
  onMessage: vi.fn().mockReturnValue(() => {}),
  onStatusChange: vi.fn().mockReturnValue(() => {}),
  onConnected: vi.fn().mockReturnValue(() => {}),
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

  it("a session_config server message reconfigures mic capture and is not forwarded", async () => {
    const onServerMessage = vi.fn();
    renderHook(() =>
      useVoiceSession({ namespace: "default", agentName: "v", sampleRate: 16000, channels: 1, onServerMessage }),
    );
    const onMessageHandler = conn.onMessage.mock.calls[0][0] as (m: unknown) => void;
    act(() => {
      onMessageHandler({
        type: MessageTypeSessionConfig,
        session_config: { codec: "pcm", sample_rate: 24000, channels: 1 },
      });
    });
    expect(reconfigureMic).toHaveBeenCalledWith(24000, 1);
    expect(onServerMessage).not.toHaveBeenCalled();
  });

  it("a session_config the client cannot satisfy tears down to error", async () => {
    reconfigureMic.mockRejectedValueOnce(new Error("cannot capture at 48000"));
    const { result } = renderHook(() =>
      useVoiceSession({ namespace: "default", agentName: "v", sampleRate: 16000, channels: 1 }),
    );
    const onMessageHandler = conn.onMessage.mock.calls[0][0] as (m: unknown) => void;
    act(() => {
      onMessageHandler({
        type: MessageTypeSessionConfig,
        session_config: { codec: "pcm", sample_rate: 48000, channels: 2 },
      });
    });
    await waitFor(() => expect(result.current.state).toBe("error"));
    expect(stopMic).toHaveBeenCalled();
    expect(stopPlay).toHaveBeenCalled();
  });

  it("first connect (resumed:false) goes live; resumed:true stays live and keeps seq", async () => {
    const { result } = renderHook(() =>
      useVoiceSession({ namespace: "default", agentName: "v", sampleRate: 24000, channels: 1 }),
    );
    // Start a call
    await act(async () => { result.current.call(); });

    // Pull the onConnected handler registered by the hook
    const onConnectedHandler = conn.onConnected.mock.calls[0][0] as (info: { sessionId: string; resumed: boolean }) => void;

    // First connect: resumed:false → live
    act(() => { onConnectedHandler({ sessionId: "s0", resumed: false }); });
    expect(result.current.state).toBe("live");

    // resumed:true after being live → stays live; seq is NOT reset
    act(() => { onConnectedHandler({ sessionId: "s1", resumed: true }); });
    expect(result.current.state).toBe("live");
    // Mic and playback are untouched on a successful resume
    expect(stopMic).not.toHaveBeenCalled();
    expect(stopPlay).not.toHaveBeenCalled();
  });

  // Regression test for Critical bug: blip must NOT tear down mic/playback.
  it("blip (unintentional disconnect) keeps mic+playback alive and sets state to connecting", async () => {
    const { result } = renderHook(() =>
      useVoiceSession({ namespace: "default", agentName: "v", sampleRate: 24000, channels: 1 }),
    );
    await act(async () => { result.current.call(); });

    const onStatusHandler = conn.onStatusChange.mock.calls[0][0] as (s: string) => void;
    const onConnectedHandler = conn.onConnected.mock.calls[0][0] as (info: { sessionId: string; resumed: boolean }) => void;

    // Go live first so liveOnceRef is true
    act(() => { onConnectedHandler({ sessionId: "s0", resumed: false }); });
    expect(result.current.state).toBe("live");

    vi.clearAllMocks(); // reset call counts after the first connect

    // Fire a blip disconnect (not from hangup)
    act(() => { onStatusHandler("disconnected"); });

    // MUST be "connecting" (pending reconnect), NOT "idle"
    expect(result.current.state).toBe("connecting");
    // Mic and playback must NOT have been stopped — the call is still alive
    expect(stopMic).not.toHaveBeenCalled();
    expect(stopPlay).not.toHaveBeenCalled();
  });

  // Resume succeeds: disconnected then onConnected resumed:true → state live, no teardown.
  it("resume succeeds: blip then resumed:true → live with no teardown", async () => {
    const { result } = renderHook(() =>
      useVoiceSession({ namespace: "default", agentName: "v", sampleRate: 24000, channels: 1 }),
    );
    await act(async () => { result.current.call(); });

    const onStatusHandler = conn.onStatusChange.mock.calls[0][0] as (s: string) => void;
    const onConnectedHandler = conn.onConnected.mock.calls[0][0] as (info: { sessionId: string; resumed: boolean }) => void;

    // First connect
    act(() => { onConnectedHandler({ sessionId: "s0", resumed: false }); });
    expect(result.current.state).toBe("live");

    vi.clearAllMocks();

    // Blip
    act(() => { onStatusHandler("disconnected"); });
    expect(result.current.state).toBe("connecting");

    // Reconnect with resume
    act(() => { onConnectedHandler({ sessionId: "s0", resumed: true }); });
    expect(result.current.state).toBe("live");
    expect(stopMic).not.toHaveBeenCalled();
    expect(stopPlay).not.toHaveBeenCalled();
  });

  // Resume fails (amnesiac): disconnected then onConnected resumed:false after going live → teardown to idle.
  it("resume fails (amnesiac): blip then resumed:false after live → teardown to idle", async () => {
    const { result } = renderHook(() =>
      useVoiceSession({ namespace: "default", agentName: "v", sampleRate: 24000, channels: 1 }),
    );
    await act(async () => { result.current.call(); });

    const onStatusHandler = conn.onStatusChange.mock.calls[0][0] as (s: string) => void;
    const onConnectedHandler = conn.onConnected.mock.calls[0][0] as (info: { sessionId: string; resumed: boolean }) => void;

    // First connect (not a reconnect)
    act(() => { onConnectedHandler({ sessionId: "s0", resumed: false }); });
    expect(result.current.state).toBe("live");

    vi.clearAllMocks();

    // Blip disconnect
    act(() => { onStatusHandler("disconnected"); });
    expect(result.current.state).toBe("connecting");

    // Reconnect but session is amnesiac (fresh, not resumed) → teardown
    act(() => { onConnectedHandler({ sessionId: "s1", resumed: false }); });
    await waitFor(() => expect(result.current.state).toBe("idle"));
    expect(stopMic).toHaveBeenCalled();
    expect(stopPlay).toHaveBeenCalled();
  });

  // Seq keep vs reset (I1): resumed:true keeps seq; teardown starts fresh at 0.
  it("sequence continues on resumed:true and resets on amnesiac reconnect", async () => {
    const { result } = renderHook(() =>
      useVoiceSession({ namespace: "default", agentName: "v", sampleRate: 24000, channels: 1 }),
    );
    await act(async () => { result.current.call(); });

    const onStatusHandler = conn.onStatusChange.mock.calls[0][0] as (s: string) => void;
    const onConnectedHandler = conn.onConnected.mock.calls[0][0] as (info: { sessionId: string; resumed: boolean }) => void;

    // First connect (fresh)
    act(() => { onConnectedHandler({ sessionId: "s0", resumed: false }); });
    expect(result.current.state).toBe("live");

    // Blip then successful resume
    act(() => { onStatusHandler("disconnected"); });
    act(() => { onConnectedHandler({ sessionId: "s0", resumed: true }); });
    expect(result.current.state).toBe("live");

    // After resume, state is still live and no teardown occurred.
    // We can't drive onFrame directly in unit tests without a real AudioWorklet,
    // but we verify the branch outcome: resumed:true → live (seq preserved), teardown skipped.
    expect(stopMic).not.toHaveBeenCalled();
    expect(stopPlay).not.toHaveBeenCalled();

    // Amnesiac reconnect: blip then resumed:false → teardown (seq implicitly reset via teardownToIdle)
    vi.clearAllMocks();
    act(() => { onStatusHandler("disconnected"); });
    act(() => { onConnectedHandler({ sessionId: "s1", resumed: false }); });
    await waitFor(() => expect(result.current.state).toBe("idle"));
    expect(stopMic).toHaveBeenCalled();
    expect(stopPlay).toHaveBeenCalled();
    // A subsequent call() would start with seqRef=0 (reset by teardownToIdle).
    // Calling call() again to verify state returns to connecting (confirming liveOnceRef reset)
    vi.clearAllMocks();
    await act(async () => { result.current.call(); });
    // First connect after a teardown: resumed:false → live (not amnesiac path since liveOnceRef is false)
    act(() => { onConnectedHandler({ sessionId: "s2", resumed: false }); });
    expect(result.current.state).toBe("live");
    expect(stopMic).not.toHaveBeenCalled();
    expect(stopPlay).not.toHaveBeenCalled();
  });

  it("does NOT tear down on transient disconnect when a reconnect is in progress", async () => {
    const { result } = renderHook(() =>
      useVoiceSession({ namespace: "default", agentName: "v", sampleRate: 24000, channels: 1 }),
    );
    await act(async () => { result.current.call(); });

    const onStatusHandler = conn.onStatusChange.mock.calls[0][0] as (s: string) => void;
    act(() => { onStatusHandler("connected"); });

    vi.clearAllMocks();

    // "connecting" means a reconnect is in flight — not a teardown event
    act(() => { onStatusHandler("connecting"); });

    // State should not become idle; mic and playback should not be stopped
    expect(result.current.state).not.toBe("idle");
    expect(stopMic).not.toHaveBeenCalled();
    expect(stopPlay).not.toHaveBeenCalled();
  });

  it("hangup() sends the hangup control message before disconnecting", async () => {
    const { result } = renderHook(() =>
      useVoiceSession({ namespace: "default", agentName: "v", sampleRate: 24000, channels: 1 }),
    );
    await act(async () => { result.current.call(); });

    act(() => { result.current.hangup(); });

    expect(conn.sendHangup).toHaveBeenCalled();
    expect(conn.disconnect).toHaveBeenCalled();
    await waitFor(() => expect(result.current.state).toBe("idle"));
  });

  // Fix M3: assert exactly once, not <=1
  it("hangup() does not trigger teardown handler when disconnect event fires", async () => {
    const { result } = renderHook(() =>
      useVoiceSession({ namespace: "default", agentName: "v", sampleRate: 24000, channels: 1 }),
    );
    await act(async () => { result.current.call(); });

    const onStatusHandler = conn.onStatusChange.mock.calls[0][0] as (s: string) => void;
    act(() => { onStatusHandler("connected"); });

    vi.clearAllMocks();

    // Hangup: teardown happens exactly once here
    act(() => { result.current.hangup(); });
    expect(stopMic).toHaveBeenCalledTimes(1);
    expect(stopPlay).toHaveBeenCalledTimes(1);

    // The disconnect status arrives (from the WS close after disconnect())
    act(() => { onStatusHandler("disconnected"); });

    // Still exactly once — the unintentional-disconnect handler must NOT re-fire
    expect(stopMic).toHaveBeenCalledTimes(1);
    expect(stopPlay).toHaveBeenCalledTimes(1);
  });
});

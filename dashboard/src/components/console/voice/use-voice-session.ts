import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { LiveAgentConnection } from "../../../lib/data/live-service";
import type { ServerMessage } from "../../../types/websocket";
import { MessageTypeInterrupt, MessageTypeSessionConfig } from "../../../types/generated/websocket";
import { useMicCapture } from "./use-mic-capture";
import { useStreamingPlayback } from "./use-streaming-playback";

export type VoiceState = "idle" | "requesting-mic" | "connecting" | "live" | "error";

export interface UseVoiceSessionOptions {
  namespace: string;
  agentName: string;
  sampleRate: number;
  channels: number;
  onServerMessage?: (m: ServerMessage) => void;
}

export function useVoiceSession(opts: UseVoiceSessionOptions) {
  const { namespace, agentName, sampleRate, channels, onServerMessage } = opts;
  const [state, setState] = useState<VoiceState>("idle");
  const seqRef = useRef(0);
  // formatRef holds the active capture format used for outbound frame metadata.
  // Seeded from opts; updated when the runtime relays a session_config
  // counter-offer so the metadata matches what the mic now captures.
  const formatRef = useRef({ sampleRate, channels });
  // Tracks whether the current disconnect event was caused by an intentional hangup().
  const hangupInProgressRef = useRef(false);
  // True once the call has gone live at least once this session.
  const liveOnceRef = useRef(false);

  const conn = useMemo(
    () => new LiveAgentConnection(namespace, agentName),
    [namespace, agentName],
  );

  const playback = useStreamingPlayback({ sampleRate });

  const mic = useMicCapture({
    sampleRate,
    channels,
    onFrame: (pcm) =>
      conn.sendBinary(pcm, {
        sequence: seqRef.current++,
        isLast: false,
        sampleRate: formatRef.current.sampleRate,
        channels: formatRef.current.channels,
      }),
  });

  // Full teardown to idle — used when a reconnect fails (amnesiac fresh session).
  const teardownToIdle = useCallback(() => {
    mic.stop();
    playback.stop();
    seqRef.current = 0;
    liveOnceRef.current = false;
    setState("idle");
  }, [mic, playback]);

  // Full teardown to error — used on unintentional terminal error.
  const teardownToError = useCallback(() => {
    mic.stop();
    playback.stop();
    seqRef.current = 0;
    liveOnceRef.current = false;
    setState("error");
  }, [mic, playback]);

  const handleDisconnect = useCallback(() => {
    if (hangupInProgressRef.current) {
      // Disconnect fired because hangup() called conn.disconnect(); already torn down.
      return;
    }
    // Unintentional blip: the LiveAgentConnection will auto-reconnect.
    // Do NOT tear down mic/playback — keep capture+playback alive across the blip.
    setState("connecting");
  }, []);

  const handleConnected = useCallback(
    (resumed: boolean) => {
      if (resumed) {
        // Successful resume: keep seqRef unchanged so frames are contiguous.
        liveOnceRef.current = true;
        setState("live");
        return;
      }
      // resumed === false
      if (liveOnceRef.current) {
        // Reconnect arrived but the session is amnesiac (fresh, not resumed).
        // The call is effectively lost — teardown rather than silently continuing.
        teardownToIdle();
        return;
      }
      // First connect of this call — normal fresh session start.
      seqRef.current = 0;
      liveOnceRef.current = true;
      setState("live");
    },
    [teardownToIdle],
  );

  useEffect(() => {
    const offBin = conn.onBinaryMedia((payload, _seq, _isLast, rate) => playback.enqueue(payload, rate));
    const offMsg = conn.onMessage((m: ServerMessage) => {
      if (m.type === MessageTypeInterrupt) {
        playback.flush();
        return;
      }
      if (m.type === MessageTypeSessionConfig && m.session_config) {
        // The runtime's negotiated capture format (RuntimeHello counter-offer).
        // Update the outbound frame metadata and re-capture at the new format.
        // If the client cannot re-capture at the required format, the session
        // cannot proceed — tear down to error (client-side fail-closed).
        const { sample_rate, channels: ch } = m.session_config;
        formatRef.current = { sampleRate: sample_rate, channels: ch };
        mic.reconfigure(sample_rate, ch).catch(() => teardownToError());
        return;
      }
      onServerMessage?.(m);
    });

    const offConnected = conn.onConnected(({ resumed }) => {
      handleConnected(resumed);
    });

    const offStatus = conn.onStatusChange((status) => {
      if (status === "connected") {
        // onConnected will set state to "live"; nothing to do here.
        return;
      }
      if (status === "connecting") {
        // Transient reconnect in flight — do NOT tear down mic/playback.
        setState("connecting");
        return;
      }
      if (status === "error") {
        if (hangupInProgressRef.current) {
          // Error arrived as part of an intentional hangup; already torn down.
          return;
        }
        // Unintentional terminal error → full teardown.
        teardownToError();
        return;
      }
      if (status === "disconnected") {
        handleDisconnect();
      }
    });

    return () => {
      offBin();
      offMsg();
      offConnected();
      offStatus();
    };
  }, [conn, playback, mic, onServerMessage, handleConnected, handleDisconnect, teardownToError]);

  const call = useCallback(() => {
    // Clear the hangup flag so unintentional-disconnect teardown is re-enabled.
    hangupInProgressRef.current = false;
    liveOnceRef.current = false;
    setState("requesting-mic");
    mic
      .start()
      .then(() => {
        setState("connecting");
        conn.startAudioSession();
      })
      .catch(() => { mic.stop(); setState("error"); });
  }, [conn, mic]);

  const hangup = useCallback(() => {
    // Mark as intentional so the onStatusChange handler does not double-teardown.
    // The flag is cleared when the next call() starts.
    hangupInProgressRef.current = true;
    conn.sendHangup();
    mic.stop();
    playback.stop();
    conn.disconnect();
    seqRef.current = 0;
    liveOnceRef.current = false;
    setState("idle");
  }, [conn, mic, playback]);

  return { state, call, hangup };
}

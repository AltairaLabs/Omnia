import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { LiveAgentConnection } from "../../../lib/data/live-service";
import type { ServerMessage } from "../../../types/websocket";
import { MessageTypeInterrupt } from "../../../types/generated/websocket";
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
        sampleRate,
        channels,
      }),
  });

  useEffect(() => {
    const offBin = conn.onBinaryMedia((payload) => playback.enqueue(payload));
    const offMsg = conn.onMessage((m: ServerMessage) => {
      if (m.type === MessageTypeInterrupt) {
        playback.flush();
        return;
      }
      onServerMessage?.(m);
    });
    const offStatus = conn.onStatusChange((status) => {
      if (status === "connected") {
        setState("live");
      } else if (status === "error") {
        setState("error");
      }
    });
    return () => {
      offBin();
      offMsg();
      offStatus();
    };
  }, [conn, playback, onServerMessage]);

  const call = useCallback(() => {
    setState("requesting-mic");
    mic
      .start()
      .then(() => {
        setState("connecting");
        conn.startAudioSession();
      })
      .catch(() => setState("error"));
  }, [conn, mic]);

  const hangup = useCallback(() => {
    mic.stop();
    playback.stop();
    conn.disconnect();
    seqRef.current = 0;
    setState("idle");
  }, [conn, mic, playback]);

  return { state, call, hangup };
}

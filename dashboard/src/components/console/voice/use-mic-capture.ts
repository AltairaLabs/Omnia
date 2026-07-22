import { useCallback, useRef, useState } from "react";

export interface UseMicCaptureOptions {
  sampleRate: number;
  channels: number;
  onFrame: (pcm: ArrayBuffer) => void;
}

export function useMicCapture({ sampleRate, channels, onFrame }: UseMicCaptureOptions) {
  const ctxRef = useRef<AudioContext | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  // formatRef holds the active capture format. It is seeded from props on
  // start() and updated by reconfigure() when the runtime counter-offers a
  // different format (RuntimeHello / session_config).
  const formatRef = useRef({ sampleRate, channels });
  const [level] = useState(0);

  const stop = useCallback(() => {
    streamRef.current?.getTracks().forEach((t) => t.stop());
    streamRef.current = null;
    const closeCtx = ctxRef.current?.close();
    if (closeCtx) {
      closeCtx.catch(() => undefined);
    }
    ctxRef.current = null;
  }, []);

  // openCapture opens the mic + AudioContext at the current formatRef.
  const openCapture = useCallback(async () => {
    const { sampleRate: rate, channels: ch } = formatRef.current;
    const stream = await navigator.mediaDevices.getUserMedia({
      audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true, channelCount: ch },
    });
    streamRef.current = stream;
    const ctx = new AudioContext({ sampleRate: rate });
    ctxRef.current = ctx;
    await ctx.audioWorklet.addModule("/audio/pcm-capture-processor.js");
    const node = new AudioWorkletNode(ctx, "pcm-capture-processor");
    node.port.onmessage = (e: MessageEvent) => {
      const { pcm } = e.data as { pcm: ArrayBuffer };
      onFrame(pcm);
    };
    const src = ctx.createMediaStreamSource(stream);
    src.connect(node);
  }, [onFrame]);

  const start = useCallback(async () => {
    formatRef.current = { sampleRate, channels };
    await openCapture();
  }, [sampleRate, channels, openCapture]);

  // reconfigure applies the runtime's negotiated capture format. When it
  // differs from the active format and capture is running, it restarts capture
  // at the new sample rate / channels so subsequent frames match the runtime's
  // requirement. A no-op when the format is unchanged.
  const reconfigure = useCallback(
    async (rate: number, ch: number) => {
      if (formatRef.current.sampleRate === rate && formatRef.current.channels === ch) {
        return;
      }
      const wasRunning = ctxRef.current !== null;
      formatRef.current = { sampleRate: rate, channels: ch };
      if (wasRunning) {
        stop();
        await openCapture();
      }
    },
    [stop, openCapture],
  );

  return { start, stop, reconfigure, format: formatRef, level };
}

import { useCallback, useRef, useState } from "react";

export interface UseMicCaptureOptions {
  sampleRate: number;
  channels: number;
  onFrame: (pcm: ArrayBuffer) => void;
}

export function useMicCapture({ sampleRate, channels, onFrame }: UseMicCaptureOptions) {
  const ctxRef = useRef<AudioContext | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const [level] = useState(0);

  const start = useCallback(async () => {
    const stream = await navigator.mediaDevices.getUserMedia({
      audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true, channelCount: channels },
    });
    streamRef.current = stream;
    const ctx = new AudioContext({ sampleRate });
    ctxRef.current = ctx;
    await ctx.audioWorklet.addModule("/audio/pcm-capture-processor.js");
    const node = new AudioWorkletNode(ctx, "pcm-capture-processor");
    node.port.onmessage = (e: MessageEvent) => {
      const { pcm } = e.data as { pcm: ArrayBuffer };
      onFrame(pcm);
    };
    const src = ctx.createMediaStreamSource(stream);
    src.connect(node);
  }, [sampleRate, channels, onFrame]);

  const stop = useCallback(() => {
    streamRef.current?.getTracks().forEach((t) => t.stop());
    streamRef.current = null;
    const closeCtx = ctxRef.current?.close();
    if (closeCtx) {
      closeCtx.catch(() => undefined);
    }
    ctxRef.current = null;
  }, []);

  return { start, stop, level };
}

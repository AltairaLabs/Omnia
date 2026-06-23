import { useCallback, useRef } from "react";

export interface UseStreamingPlaybackOptions {
  sampleRate: number;
}

export function useStreamingPlayback({ sampleRate }: UseStreamingPlaybackOptions) {
  const ctxRef = useRef<AudioContext | null>(null);
  const playheadRef = useRef(0);
  const sourcesRef = useRef<AudioBufferSourceNode[]>([]);

  const ctx = useCallback(() => {
    if (!ctxRef.current) {
      ctxRef.current = new AudioContext({ sampleRate });
      playheadRef.current = ctxRef.current.currentTime;
    }
    return ctxRef.current;
  }, [sampleRate]);

  const enqueue = useCallback(
    (pcm: ArrayBuffer) => {
      const c = ctx();
      const int16 = new Int16Array(pcm);
      const buffer = c.createBuffer(1, int16.length, sampleRate);
      const channel = buffer.getChannelData(0);
      for (let i = 0; i < int16.length; i++) channel[i] = int16[i] / 0x8000;
      const source = c.createBufferSource();
      source.buffer = buffer;
      source.connect(c.destination);
      const startAt = Math.max(playheadRef.current, c.currentTime);
      source.start(startAt);
      playheadRef.current = startAt + buffer.duration;
      sourcesRef.current.push(source);
      source.onended = () => {
        sourcesRef.current = sourcesRef.current.filter((s) => s !== source);
      };
    },
    [ctx, sampleRate],
  );

  const flush = useCallback(() => {
    sourcesRef.current.forEach((s) => {
      try {
        s.stop();
      } catch {
        // already stopped — ignore
      }
    });
    sourcesRef.current = [];
    if (ctxRef.current) {
      playheadRef.current = ctxRef.current.currentTime;
    }
  }, []);

  const stop = useCallback(() => {
    flush();
    const closeCtx = ctxRef.current?.close();
    if (closeCtx) {
      closeCtx.catch(() => undefined);
    }
    ctxRef.current = null;
  }, [flush]);

  return { enqueue, flush, stop };
}

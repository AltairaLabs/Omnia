"use client";

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";

export interface UseReplayPlaybackInput {
  readonly durationMs: number;
}

export interface UseReplayPlaybackResult {
  readonly currentTimeMs: number;
  readonly playing: boolean;
  readonly speed: number;
  play: () => void;
  pause: () => void;
  toggle: () => void;
  seek: (ms: number) => void;
  setSpeed: (speed: number) => void;
}

function clamp(value: number, min: number, max: number): number {
  if (value < min) return min;
  if (value > max) return max;
  return value;
}

/**
 * Playback state for the session replay viewer.
 *
 * Uses a requestAnimationFrame loop while `playing` so the scrubber
 * renders at display refresh rate without tight React re-renders
 * between frames. Auto-pauses on reaching durationMs.
 *
 * The rAF loop is driven by a stable ref (`tickRef`) that is updated
 * via useLayoutEffect so the loop always calls the latest closure
 * without creating a self-referential useCallback (which ESLint
 * react-hooks/immutability disallows).
 */
export function useReplayPlayback({ durationMs }: UseReplayPlaybackInput): UseReplayPlaybackResult {
  const [currentTimeMs, setCurrentTimeMs] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [speed, setSpeedState] = useState(1);

  const frameRef = useRef<number | null>(null);
  const lastTickRef = useRef<number | null>(null);
  const speedRef = useRef(1);
  const durationRef = useRef(durationMs);
  const tickRef = useRef<FrameRequestCallback>(() => {});

  useEffect(() => {
    durationRef.current = durationMs;
  }, [durationMs]);

  const stopLoop = useCallback(() => {
    if (frameRef.current !== null) {
      cancelAnimationFrame(frameRef.current);
      frameRef.current = null;
    }
    lastTickRef.current = null;
  }, []);

  // tickImpl is defined as a stable ref value — updated in useLayoutEffect below.
  // The rAF callback calls tickRef.current so it always uses the latest closure
  // without needing the callback to reference itself.
  const scheduleNextFrame = useCallback(() => {
    frameRef.current = requestAnimationFrame((ts) => tickRef.current(ts));
  }, []);

  const tick = useCallback(
    (nowMs: number) => {
      const last = lastTickRef.current ?? nowMs;
      const deltaReal = nowMs - last;
      lastTickRef.current = nowMs;
      const deltaPlayback = deltaReal * speedRef.current;
      setCurrentTimeMs((prev) => {
        const next = prev + deltaPlayback;
        if (next >= durationRef.current) {
          setPlaying(false);
          stopLoop();
          return durationRef.current;
        }
        return next;
      });
      scheduleNextFrame();
    },
    [stopLoop, scheduleNextFrame],
  );

  // Keep tickRef current after every render so the rAF loop calls the latest closure.
  useLayoutEffect(() => {
    tickRef.current = tick;
  });

  const play = useCallback(() => {
    setCurrentTimeMs((prev) => (prev >= durationRef.current ? 0 : prev));
    setPlaying(true);
  }, []);

  const pause = useCallback(() => setPlaying(false), []);

  const toggle = useCallback(() => {
    setPlaying((p) => !p);
  }, []);

  const seek = useCallback((ms: number) => {
    setCurrentTimeMs(clamp(ms, 0, durationRef.current));
  }, []);

  const setSpeed = useCallback((nextSpeed: number) => {
    speedRef.current = nextSpeed;
    setSpeedState(nextSpeed);
  }, []);

  useEffect(() => {
    if (!playing) {
      stopLoop();
      return;
    }
    scheduleNextFrame();
    return stopLoop;
  }, [playing, scheduleNextFrame, stopLoop]);

  return { currentTimeMs, playing, speed, play, pause, toggle, seek, setSpeed };
}

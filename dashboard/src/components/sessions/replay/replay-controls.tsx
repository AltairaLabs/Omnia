"use client";

import { Play, Pause, SkipBack, SkipForward } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { toElapsedMs } from "@/lib/sessions/replay";
import type { TimelineEvent } from "@/lib/sessions/timeline";

interface ReplayControlsProps {
  readonly playing: boolean;
  readonly speed: number;
  readonly currentTimeMs: number;
  readonly durationMs: number;
  readonly startedAt: string;
  readonly events: readonly TimelineEvent[];
  onPlay: () => void;
  onPause: () => void;
  onSeek: (ms: number) => void;
  onSpeedChange: (speed: number) => void;
}

const SPEEDS = [0.5, 1, 2, 4, 10] as const;

function formatMs(ms: number): string {
  const clamped = Math.max(0, ms);
  const totalSeconds = Math.floor(clamped / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  const millis = Math.floor(clamped % 1000);
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}.${String(millis).padStart(3, "0")}`;
}

function nextEventMs(
  events: readonly TimelineEvent[],
  startedAt: string,
  afterMs: number,
): number | null {
  let candidate: number | null = null;
  for (const e of events) {
    const ms = toElapsedMs(startedAt, e.timestamp);
    if (ms > afterMs && (candidate === null || ms < candidate)) candidate = ms;
  }
  return candidate;
}

function prevEventMs(
  events: readonly TimelineEvent[],
  startedAt: string,
  beforeMs: number,
): number | null {
  let candidate: number | null = null;
  for (const e of events) {
    const ms = toElapsedMs(startedAt, e.timestamp);
    if (ms < beforeMs && (candidate === null || ms > candidate)) candidate = ms;
  }
  return candidate;
}

export function ReplayControls({
  playing,
  speed,
  currentTimeMs,
  durationMs,
  startedAt,
  events,
  onPlay,
  onPause,
  onSeek,
  onSpeedChange,
}: ReplayControlsProps) {
  const handlePrev = () => {
    const prev = prevEventMs(events, startedAt, currentTimeMs);
    if (prev !== null) onSeek(prev);
  };

  const handleNext = () => {
    const next = nextEventMs(events, startedAt, currentTimeMs);
    if (next !== null) onSeek(next);
  };

  return (
    <div className="flex items-center gap-3 border-b bg-muted/30 px-4 py-2">
      <Button
        variant="ghost"
        size="sm"
        onClick={handlePrev}
        aria-label="Previous event"
      >
        <SkipBack className="h-4 w-4" />
      </Button>
      {playing ? (
        <Button variant="secondary" size="sm" onClick={onPause} aria-label="Pause">
          <Pause className="h-4 w-4" />
        </Button>
      ) : (
        <Button variant="secondary" size="sm" onClick={onPlay} aria-label="Play">
          <Play className="h-4 w-4" />
        </Button>
      )}
      <Button
        variant="ghost"
        size="sm"
        onClick={handleNext}
        aria-label="Next event"
      >
        <SkipForward className="h-4 w-4" />
      </Button>
      <Select value={String(speed)} onValueChange={(v) => onSpeedChange(Number(v))}>
        <SelectTrigger className="h-8 w-20" aria-label="Playback speed">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {SPEEDS.map((s) => (
            <SelectItem key={s} value={String(s)}>
              {s}x
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <span className="ml-auto font-mono text-xs text-muted-foreground">
        {formatMs(currentTimeMs)} / {formatMs(durationMs)}
      </span>
    </div>
  );
}

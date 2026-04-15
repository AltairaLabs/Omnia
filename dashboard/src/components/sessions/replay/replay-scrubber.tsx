"use client";

import { Slider } from "@/components/ui/slider";
import { toElapsedMs } from "@/lib/sessions/replay";
import type { TimelineEvent, TimelineEventKind } from "@/lib/sessions/timeline";
import { cn } from "@/lib/utils";

interface ReplayScrubberProps {
  readonly startedAt: string;
  readonly durationMs: number;
  readonly currentTimeMs: number;
  readonly events: readonly TimelineEvent[];
  readonly onSeek: (ms: number) => void;
}

const MARKER_COLOR: Record<TimelineEventKind, string> = {
  user_message: "bg-primary",
  assistant_message: "bg-blue-500",
  system_message: "bg-gray-400",
  tool_call: "bg-orange-500",
  tool_result: "bg-amber-500",
  pipeline_event: "bg-indigo-500",
  stage_event: "bg-cyan-500",
  provider_call: "bg-yellow-500",
  workflow_transition: "bg-purple-500",
  workflow_completed: "bg-green-500",
  eval_event: "bg-violet-500",
  error: "bg-destructive",
};

export function ReplayScrubber({
  startedAt,
  durationMs,
  currentTimeMs,
  events,
  onSeek,
}: ReplayScrubberProps) {
  const safeDuration = durationMs > 0 ? durationMs : 1;
  return (
    <div className="relative w-full py-2">
      <div className="relative h-2 w-full">
        {events.map((e) => {
          const elapsed = toElapsedMs(startedAt, e.timestamp);
          const pct = Math.min(100, (elapsed / safeDuration) * 100);
          return (
            <span
              key={e.id}
              data-event-marker
              title={`${e.label} (${(elapsed / 1000).toFixed(2)}s)`}
              className={cn(
                "absolute top-1/2 h-2 w-2 -translate-x-1/2 -translate-y-1/2 rounded-full",
                MARKER_COLOR[e.kind],
              )}
              style={{ left: `${pct}%` }}
            />
          );
        })}
      </div>
      <Slider
        value={[currentTimeMs]}
        min={0}
        max={safeDuration}
        step={10}
        onValueChange={(values) => onSeek(values[0])}
        aria-label="Session replay timeline"
        className="mt-2"
      />
    </div>
  );
}

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

// Categorical markers differentiating timeline event kinds. Mapped onto the
// SI-tunable --category-1..8 token palette (nearest hue) so markers re-theme
// under white-label branding. primary/destructive are already design tokens.
const MARKER_COLOR: Record<TimelineEventKind, string> = {
  user_message: "bg-primary",
  assistant_message: "bg-category-1",
  system_message: "bg-category-8",
  tool_call: "bg-category-4",
  tool_result: "bg-category-4",
  pipeline_event: "bg-category-2",
  stage_event: "bg-category-6",
  provider_call: "bg-category-4",
  workflow_transition: "bg-category-2",
  workflow_completed: "bg-category-5",
  eval_event: "bg-category-2",
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

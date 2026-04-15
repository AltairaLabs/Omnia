"use client";

import { useMemo } from "react";
import { Badge } from "@/components/ui/badge";
import { toElapsedMs } from "@/lib/sessions/replay";
import type { TimelineEvent } from "@/lib/sessions/timeline";

interface ReplayEventDetailProps {
  readonly startedAt: string;
  readonly currentTimeMs: number;
  readonly events: readonly TimelineEvent[];
}

function currentEvent(
  startedAt: string,
  currentTimeMs: number,
  events: readonly TimelineEvent[],
): TimelineEvent | null {
  let latest: TimelineEvent | null = null;
  let latestMs = -1;
  for (const e of events) {
    const ms = toElapsedMs(startedAt, e.timestamp);
    if (ms <= currentTimeMs && ms > latestMs) {
      latest = e;
      latestMs = ms;
    }
  }
  return latest;
}

export function ReplayEventDetail({ startedAt, currentTimeMs, events }: ReplayEventDetailProps) {
  const event = useMemo(
    () => currentEvent(startedAt, currentTimeMs, events),
    [startedAt, currentTimeMs, events],
  );
  if (!event) {
    return (
      <div className="flex h-full items-center justify-center rounded-md border text-sm text-muted-foreground">
        No event yet — press play or scrub forward.
      </div>
    );
  }
  return (
    <div className="flex h-full flex-col gap-2 rounded-md border p-3">
      <div className="flex items-center gap-2">
        <Badge variant="outline">{event.kind}</Badge>
        <span className="font-mono text-xs text-muted-foreground">
          {(toElapsedMs(startedAt, event.timestamp) / 1000).toFixed(3)}s
        </span>
      </div>
      <div className="text-sm font-semibold">{event.label}</div>
      {event.detail && <div className="whitespace-pre-wrap break-words text-sm">{event.detail}</div>}
      {event.metadata && (
        <pre className="overflow-x-auto rounded bg-muted p-2 text-xs">
          {JSON.stringify(event.metadata, null, 2)}
        </pre>
      )}
    </div>
  );
}

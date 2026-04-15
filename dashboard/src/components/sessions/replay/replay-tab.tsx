"use client";

import { useMemo, useState } from "react";
import { PanelRightClose, PanelRightOpen } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ReplayControls } from "./replay-controls";
import { ReplayScrubber } from "./replay-scrubber";
import { ReplayMetrics } from "./replay-metrics";
import { ReplayConversation } from "./replay-conversation";
import { ReplayDetails } from "./replay-details";
import { useReplayPlayback } from "@/hooks/use-replay-playback";
import { sessionDurationMs } from "@/lib/sessions/replay";
import { extractTimelineEvents } from "@/lib/sessions/timeline";
import { cn } from "@/lib/utils";
import type { Session, Message, ToolCall, ProviderCall, RuntimeEvent } from "@/types/session";

interface ReplayTabProps {
  readonly session: Session;
  readonly messages: readonly Message[];
  readonly toolCalls: readonly ToolCall[];
  readonly providerCalls: readonly ProviderCall[];
  readonly runtimeEvents: readonly RuntimeEvent[];
}

export function ReplayTab({
  session,
  messages,
  toolCalls,
  providerCalls,
  runtimeEvents,
}: ReplayTabProps) {
  const [detailsOpen, setDetailsOpen] = useState(false);

  const timeline = useMemo(
    () =>
      extractTimelineEvents(
        messages as Message[],
        toolCalls as ToolCall[],
        providerCalls as ProviderCall[],
        runtimeEvents as RuntimeEvent[],
      ),
    [messages, toolCalls, providerCalls, runtimeEvents],
  );

  const durationMs = useMemo(
    () => sessionDurationMs(session.startedAt, timeline.map((e) => e.timestamp)),
    [session.startedAt, timeline],
  );

  const { currentTimeMs, playing, speed, play, pause, seek, setSpeed } = useReplayPlayback({
    durationMs,
  });

  if (timeline.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        Nothing to replay — this session has no recorded events.
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      <ReplayControls
        playing={playing}
        speed={speed}
        currentTimeMs={currentTimeMs}
        durationMs={durationMs}
        startedAt={session.startedAt}
        events={timeline}
        onPlay={play}
        onPause={pause}
        onSeek={seek}
        onSpeedChange={setSpeed}
      />
      <div className="border-b px-4 py-2">
        <ReplayScrubber
          startedAt={session.startedAt}
          durationMs={durationMs}
          currentTimeMs={currentTimeMs}
          events={timeline}
          onSeek={seek}
        />
      </div>
      <ReplayMetrics
        startedAt={session.startedAt}
        currentTimeMs={currentTimeMs}
        messages={messages}
        toolCalls={toolCalls}
        providerCalls={providerCalls}
      />

      {/* Main + drawer */}
      <div className="relative flex flex-1 min-h-0">
        <div className="flex flex-1 min-w-0 flex-col p-4">
          <div className="mb-2 flex items-center justify-end">
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setDetailsOpen((v) => !v)}
              aria-label={detailsOpen ? "Hide details" : "Show details"}
              aria-expanded={detailsOpen}
              className="h-7 gap-1.5 text-xs"
            >
              {detailsOpen ? (
                <PanelRightClose className="h-3.5 w-3.5" />
              ) : (
                <PanelRightOpen className="h-3.5 w-3.5" />
              )}
              Details
            </Button>
          </div>
          <div className="flex-1 min-h-0">
            <ReplayConversation
              startedAt={session.startedAt}
              currentTimeMs={currentTimeMs}
              messages={messages}
              toolCalls={toolCalls}
            />
          </div>
        </div>

        <aside
          data-testid="replay-details-drawer"
          aria-hidden={!detailsOpen}
          className={cn(
            "flex-shrink-0 overflow-hidden border-l bg-background transition-[width] duration-200 ease-out",
            detailsOpen ? "w-96" : "w-0",
          )}
        >
          <div className="h-full w-96 p-4">
            <ReplayDetails
              startedAt={session.startedAt}
              currentTimeMs={currentTimeMs}
              events={timeline}
            />
          </div>
        </aside>
      </div>
    </div>
  );
}

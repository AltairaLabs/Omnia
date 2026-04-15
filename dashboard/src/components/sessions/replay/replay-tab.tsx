"use client";

import { useMemo } from "react";
import { ReplayControls } from "./replay-controls";
import { ReplayScrubber } from "./replay-scrubber";
import { ReplayMetrics } from "./replay-metrics";
import { ReplayConversation } from "./replay-conversation";
import { ReplayDetails } from "./replay-details";
import { useReplayPlayback } from "@/hooks/use-replay-playback";
import { sessionDurationMs } from "@/lib/sessions/replay";
import { extractTimelineEvents } from "@/lib/sessions/timeline";
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
      <div className="grid flex-1 min-h-0 grid-cols-1 gap-3 p-4 md:grid-cols-[2fr_1fr]">
        <ReplayConversation
          startedAt={session.startedAt}
          currentTimeMs={currentTimeMs}
          messages={messages}
          toolCalls={toolCalls}
        />
        <ReplayDetails
          startedAt={session.startedAt}
          currentTimeMs={currentTimeMs}
          events={timeline}
        />
      </div>
    </div>
  );
}

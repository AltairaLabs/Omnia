"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import {
  ChevronRight,
  ChevronDown,
  User,
  Bot,
  MessageSquare,
  Wrench,
  GitBranch,
  Layers,
  Cpu,
  Zap,
  ArrowLeftRight,
  Shield,
  CheckCircle2,
  AlertCircle,
} from "lucide-react";
import { toElapsedMs } from "@/lib/sessions/replay";
import type { TimelineEvent, TimelineEventKind } from "@/lib/sessions/timeline";
import { cn } from "@/lib/utils";

interface ReplayDetailsProps {
  readonly startedAt: string;
  readonly currentTimeMs: number;
  readonly events: readonly TimelineEvent[];
}

const KIND_ICON: Record<TimelineEventKind, React.ReactNode> = {
  user_message: <User className="h-3 w-3" />,
  assistant_message: <Bot className="h-3 w-3" />,
  system_message: <MessageSquare className="h-3 w-3" />,
  tool_call: <Wrench className="h-3 w-3" />,
  tool_result: <ArrowLeftRight className="h-3 w-3" />,
  pipeline_event: <Layers className="h-3 w-3" />,
  stage_event: <Cpu className="h-3 w-3" />,
  provider_call: <Zap className="h-3 w-3" />,
  workflow_transition: <GitBranch className="h-3 w-3" />,
  workflow_completed: <CheckCircle2 className="h-3 w-3" />,
  eval_event: <Shield className="h-3 w-3" />,
  error: <AlertCircle className="h-3 w-3" />,
};

const KIND_COLOR: Record<TimelineEventKind, string> = {
  user_message: "text-primary",
  assistant_message: "text-blue-500",
  system_message: "text-gray-500",
  tool_call: "text-orange-500",
  tool_result: "text-amber-500",
  pipeline_event: "text-indigo-500",
  stage_event: "text-cyan-500",
  provider_call: "text-yellow-600",
  workflow_transition: "text-purple-500",
  workflow_completed: "text-green-500",
  eval_event: "text-violet-500",
  error: "text-destructive",
};

function formatElapsed(ms: number): string {
  const totalSeconds = Math.floor(ms / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  const millis = Math.floor(ms % 1000);
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}.${String(millis).padStart(3, "0")}`;
}

interface LogRowProps {
  readonly event: TimelineEvent;
  readonly elapsedMs: number;
  readonly isCurrent: boolean;
  readonly expanded: boolean;
  onToggle: () => void;
}

function LogRow({ event, elapsedMs, isCurrent, expanded, onToggle }: LogRowProps) {
  return (
    <div
      data-testid="replay-details-row"
      data-current={isCurrent || undefined}
      className={cn(
        "border-b last:border-b-0 transition-colors",
        isCurrent && "bg-primary/5",
      )}
    >
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={expanded}
        className="flex w-full cursor-pointer items-center gap-2 px-3 py-1.5 text-left text-xs hover:bg-muted/50"
      >
        <span className="w-3 flex-shrink-0 text-muted-foreground">
          {expanded ? (
            <ChevronDown className="h-3 w-3" />
          ) : (
            <ChevronRight className="h-3 w-3" />
          )}
        </span>
        <span className={cn("flex-shrink-0", KIND_COLOR[event.kind])}>
          {KIND_ICON[event.kind]}
        </span>
        <span className="font-mono text-muted-foreground flex-shrink-0 tabular-nums">
          {formatElapsed(elapsedMs)}
        </span>
        <span className="truncate">{event.label}</span>
        {event.status === "error" && (
          <span className="ml-auto flex-shrink-0 rounded bg-destructive/10 px-1.5 py-0.5 text-[10px] font-medium text-destructive">
            error
          </span>
        )}
        {event.duration !== undefined && (
          <span className="ml-auto flex-shrink-0 font-mono text-muted-foreground">
            {event.duration}ms
          </span>
        )}
      </button>
      {expanded && (
        <div className="space-y-2 border-t bg-muted/30 px-10 py-2 text-xs">
          <div className="grid grid-cols-[max-content_1fr] gap-x-3 gap-y-0.5 text-[11px]">
            <span className="text-muted-foreground">kind</span>
            <span className="font-mono">{event.kind}</span>
            <span className="text-muted-foreground">id</span>
            <span className="font-mono break-all">{event.id}</span>
            <span className="text-muted-foreground">timestamp</span>
            <span className="font-mono">{event.timestamp}</span>
            {event.status && (
              <>
                <span className="text-muted-foreground">status</span>
                <span className="font-mono">{event.status}</span>
              </>
            )}
            {event.toolCallId && (
              <>
                <span className="text-muted-foreground">toolCallId</span>
                <span className="font-mono break-all">{event.toolCallId}</span>
              </>
            )}
          </div>
          {event.detail && (
            <div className="whitespace-pre-wrap break-words">{event.detail}</div>
          )}
          {event.metadata && Object.keys(event.metadata).length > 0 && (
            <pre className="overflow-x-auto rounded bg-background p-2 font-mono">
              {JSON.stringify(event.metadata, null, 2)}
            </pre>
          )}
        </div>
      )}
    </div>
  );
}

/**
 * Scrollable DevTools-style event log. Shows every timeline event up to the
 * playhead with a colored kind icon, elapsed timestamp, and an expandable
 * details drawer for `detail`/`metadata` payloads. The most-recent row is
 * highlighted and auto-scrolled into view.
 */
export function ReplayDetails({ startedAt, currentTimeMs, events }: ReplayDetailsProps) {
  const visible = useMemo(() => {
    const rows: { event: TimelineEvent; elapsedMs: number }[] = [];
    for (const e of events) {
      const ms = toElapsedMs(startedAt, e.timestamp);
      if (ms <= currentTimeMs) rows.push({ event: e, elapsedMs: ms });
    }
    rows.sort((a, b) => a.elapsedMs - b.elapsedMs);
    return rows;
  }, [events, startedAt, currentTimeMs]);

  const currentId = visible.length > 0 ? visible[visible.length - 1].event.id : null;
  const [expandedIds, setExpandedIds] = useState<ReadonlySet<string>>(new Set());

  const toggle = (id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const currentRowRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    currentRowRef.current?.scrollIntoView({ block: "nearest" });
  }, [currentId]);

  if (visible.length === 0) {
    return (
      <div className="flex h-full items-center justify-center rounded-md border text-sm text-muted-foreground">
        No events yet — press play or scrub forward.
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col rounded-md border">
      <div className="border-b bg-muted/30 px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
        Details ({visible.length})
      </div>
      <div className="flex-1 overflow-y-auto">
        {visible.map(({ event, elapsedMs }) => (
          <div
            key={event.id}
            ref={event.id === currentId ? currentRowRef : undefined}
          >
            <LogRow
              event={event}
              elapsedMs={elapsedMs}
              isCurrent={event.id === currentId}
              expanded={expandedIds.has(event.id)}
              onToggle={() => toggle(event.id)}
            />
          </div>
        ))}
      </div>
    </div>
  );
}

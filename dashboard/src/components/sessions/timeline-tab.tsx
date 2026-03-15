"use client";

import { useState } from "react";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Badge } from "@/components/ui/badge";
import { useDebugPanelStore } from "@/stores/debug-panel-store";
import { extractTimelineEvents, type TimelineEvent, type TimelineEventKind } from "@/lib/sessions/timeline";
import {
  User,
  Bot,
  MessageSquare,
  Wrench,
  GitBranch,
  CheckCircle2,
  AlertCircle,
  Layers,
  Cpu,
  Zap,
  ArrowLeftRight,
  Shield,
  ChevronDown,
  ChevronRight,
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { Message, ToolCall, ProviderCall, RuntimeEvent } from "@/types/session";

interface TimelineTabProps {
  readonly messages: Message[];
  readonly toolCalls?: ToolCall[];
  readonly providerCalls?: ProviderCall[];
  readonly runtimeEvents?: RuntimeEvent[];
}

const KIND_CONFIG: Record<TimelineEventKind, {
  icon: React.ReactNode;
  color: string;
}> = {
  user_message: {
    icon: <User className="h-3.5 w-3.5" />,
    color: "text-primary",
  },
  assistant_message: {
    icon: <Bot className="h-3.5 w-3.5" />,
    color: "text-blue-500",
  },
  system_message: {
    icon: <MessageSquare className="h-3.5 w-3.5" />,
    color: "text-gray-500",
  },
  tool_call: {
    icon: <Wrench className="h-3.5 w-3.5" />,
    color: "text-orange-500",
  },
  tool_result: {
    icon: <ArrowLeftRight className="h-3.5 w-3.5" />,
    color: "text-amber-500",
  },
  pipeline_event: {
    icon: <Layers className="h-3.5 w-3.5" />,
    color: "text-indigo-500",
  },
  stage_event: {
    icon: <Cpu className="h-3.5 w-3.5" />,
    color: "text-cyan-500",
  },
  provider_call: {
    icon: <Zap className="h-3.5 w-3.5" />,
    color: "text-yellow-500",
  },
  workflow_transition: {
    icon: <GitBranch className="h-3.5 w-3.5" />,
    color: "text-purple-500",
  },
  eval_event: {
    icon: <Shield className="h-3.5 w-3.5" />,
    color: "text-violet-500",
  },
  workflow_completed: {
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
    color: "text-green-500",
  },
  error: {
    icon: <AlertCircle className="h-3.5 w-3.5" />,
    color: "text-destructive",
  },
};

function formatTimestamp(iso: string): string {
  try {
    const date = new Date(iso);
    return date.toISOString().slice(11, 23); // HH:mm:ss.SSS
  } catch {
    return iso;
  }
}

/** A group of events between pipeline.started and pipeline.completed (or end of list). */
interface PipelineGroup {
  type: "pipeline";
  startEvent: TimelineEvent;
  endEvent?: TimelineEvent;
  children: TimelineEvent[];
}

/** A group of eval events sharing the same trigger type. */
interface EvalGroup {
  type: "eval_group";
  trigger: string;
  children: TimelineEvent[];
  timestamp: string;
}

type TimelineItem =
  | { type: "event"; event: TimelineEvent }
  | PipelineGroup
  | EvalGroup;

/** Accumulate a single eval event into the pending map. */
function accumulateEvalEvent(
  event: TimelineEvent,
  pending: Map<string, TimelineEvent[]>,
) {
  const trigger = event.metadata?.trigger || "unknown";
  const existing = pending.get(trigger);
  if (existing) {
    existing.push(event);
  } else {
    pending.set(trigger, [event]);
  }
}

/** Flush pending eval groups into the result array. */
function flushPendingEvals(
  pending: Map<string, TimelineEvent[]>,
  timestamp: string,
  result: TimelineItem[],
) {
  for (const [trigger, children] of pending) {
    result.push({ type: "eval_group", trigger, children, timestamp });
  }
}

/** Collect consecutive eval events into trigger-based groups. */
function groupEvalEvents(items: TimelineItem[]): TimelineItem[] {
  const result: TimelineItem[] = [];
  let pendingEvals = new Map<string, TimelineEvent[]>();
  let pendingTimestamp = "";

  for (const item of items) {
    if (item.type === "event" && item.event.kind === "eval_event") {
      if (!pendingTimestamp) pendingTimestamp = item.event.timestamp;
      accumulateEvalEvent(item.event, pendingEvals);
    } else {
      if (pendingEvals.size > 0) {
        flushPendingEvals(pendingEvals, pendingTimestamp, result);
        pendingEvals = new Map();
        pendingTimestamp = "";
      }
      result.push(item);
    }
  }
  if (pendingEvals.size > 0) {
    flushPendingEvals(pendingEvals, pendingTimestamp, result);
  }

  return result;
}

/** Group events into top-level items and collapsible pipeline/eval sections. */
function groupIntoPipelines(events: TimelineEvent[]): TimelineItem[] {
  const items: TimelineItem[] = [];
  let currentGroup: PipelineGroup | null = null;

  for (const event of events) {
    const isPipelineStart = event.kind === "pipeline_event" && event.metadata?.type === "pipeline.started";
    const isPipelineEnd = event.kind === "pipeline_event" && event.metadata?.type === "pipeline.completed";

    if (isPipelineStart) {
      // Close any unclosed group
      if (currentGroup) items.push(currentGroup);
      currentGroup = { type: "pipeline", startEvent: event, children: [] };
    } else if (isPipelineEnd && currentGroup) {
      currentGroup.endEvent = event;
      items.push(currentGroup);
      currentGroup = null;
    } else if (currentGroup) {
      currentGroup.children.push(event);
    } else {
      items.push({ type: "event", event });
    }
  }
  // Close unclosed group (pipeline started but never completed)
  if (currentGroup) items.push(currentGroup);

  // Second pass: group consecutive eval events by trigger
  return groupEvalEvents(items);
}

function EventRow({ event, openToolCall, indent }: {
  readonly event: TimelineEvent;
  readonly openToolCall: (id: string) => void;
  readonly indent?: boolean;
}) {
  const config = KIND_CONFIG[event.kind];
  const isClickable = (event.kind === "tool_call" || event.kind === "tool_result") && event.toolCallId;

  return (
    <button
      key={event.id}
      type="button"
      className={cn(
        "flex items-center gap-3 w-full text-left px-2 py-1.5 rounded text-sm",
        "hover:bg-muted/50 transition-colors",
        isClickable && "cursor-pointer",
        !isClickable && "cursor-default",
        indent && "pl-6"
      )}
      onClick={() => {
        if (isClickable) openToolCall(event.toolCallId!);
      }}
      data-testid={`timeline-event-${event.id}`}
    >
      <span className="text-xs text-muted-foreground font-mono shrink-0 w-20">
        {formatTimestamp(event.timestamp)}
      </span>
      <span className={cn("shrink-0", config.color)}>
        {config.icon}
      </span>
      <span className="font-medium truncate shrink-0 max-w-48">
        {event.label}
      </span>
      {event.detail && (
        <span className="text-muted-foreground truncate text-xs flex-1 min-w-0">
          {event.detail}
        </span>
      )}
      <span className="flex items-center gap-1.5 shrink-0 ml-auto">
        {event.kind === "tool_call" && event.metadata?.handler_type && (
          <Badge variant="outline" className="text-xs px-1 py-0 font-mono">{event.metadata.handler_type}</Badge>
        )}
        {event.duration !== undefined && (
          <span className="text-xs text-muted-foreground">{event.duration}ms</span>
        )}
        {event.status === "success" && (
          <Badge variant="secondary" className="text-xs px-1 py-0">OK</Badge>
        )}
        {event.status === "error" && (
          <Badge variant="destructive" className="text-xs px-1 py-0">Err</Badge>
        )}
        {event.status === "pending" && (
          <Badge variant="outline" className="text-xs px-1 py-0">...</Badge>
        )}
      </span>
    </button>
  );
}

function PipelineSection({ group, openToolCall }: {
  readonly group: PipelineGroup;
  readonly openToolCall: (id: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const durationMs = group.endEvent?.duration;
  const status = group.endEvent ? group.endEvent.status : "pending";

  return (
    <div data-testid={`pipeline-group-${group.startEvent.id}`}>
      <button
        type="button"
        className={cn(
          "flex items-center gap-3 w-full text-left px-2 py-1.5 rounded text-sm",
          "hover:bg-muted/50 transition-colors cursor-pointer"
        )}
        onClick={() => setExpanded(!expanded)}
      >
        <span className="text-xs text-muted-foreground font-mono shrink-0 w-20">
          {formatTimestamp(group.startEvent.timestamp)}
        </span>
        <span className="shrink-0 text-indigo-500">
          {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
        </span>
        <span className="font-medium truncate shrink-0 max-w-48">
          Agent Pipeline
        </span>
        <span className="text-muted-foreground text-xs">
          {group.children.length} events
        </span>
        <span className="flex items-center gap-1.5 shrink-0 ml-auto">
          {durationMs !== undefined && (
            <span className="text-xs text-muted-foreground">{durationMs}ms</span>
          )}
          {status === "success" && (
            <Badge variant="secondary" className="text-xs px-1 py-0">OK</Badge>
          )}
          {status === "error" && (
            <Badge variant="destructive" className="text-xs px-1 py-0">Err</Badge>
          )}
          {status === "pending" && (
            <Badge variant="outline" className="text-xs px-1 py-0">...</Badge>
          )}
        </span>
      </button>
      {expanded && (
        <div className="border-l-2 border-indigo-500/20 ml-[6.5rem]">
          {group.children.map((child) => (
            <EventRow key={child.id} event={child} openToolCall={openToolCall} indent />
          ))}
        </div>
      )}
    </div>
  );
}

function triggerLabel(trigger: string): string {
  if (trigger === "every_turn") return "Turn";
  if (trigger === "on_session_complete") return "Session";
  return trigger;
}

function EvalGroupSection({ group, openToolCall }: {
  readonly group: EvalGroup;
  readonly openToolCall: (id: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const passed = group.children.filter((e) => e.status === "success").length;
  const failed = group.children.length - passed;

  return (
    <div data-testid={`eval-group-${group.trigger}`}>
      <button
        type="button"
        className={cn(
          "flex items-center gap-3 w-full text-left px-2 py-1.5 rounded text-sm",
          "hover:bg-muted/50 transition-colors cursor-pointer"
        )}
        onClick={() => setExpanded(!expanded)}
      >
        <span className="text-xs text-muted-foreground font-mono shrink-0 w-20">
          {formatTimestamp(group.timestamp)}
        </span>
        <span className="shrink-0 text-violet-500">
          {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
        </span>
        <span className="font-medium truncate shrink-0 max-w-48">
          Evals: {triggerLabel(group.trigger)}
        </span>
        <span className="flex items-center gap-1.5 shrink-0 ml-auto">
          <span className="text-muted-foreground text-xs">{group.children.length} evals</span>
          {passed > 0 && (
            <Badge variant="secondary" className="text-xs px-1 py-0">{passed} OK</Badge>
          )}
          {failed > 0 && (
            <Badge variant="destructive" className="text-xs px-1 py-0">{failed} Fail</Badge>
          )}
        </span>
      </button>
      {expanded && (
        <div className="border-l-2 border-violet-500/20 ml-[6.5rem]">
          {group.children.map((child) => (
            <EventRow key={child.id} event={child} openToolCall={openToolCall} indent />
          ))}
        </div>
      )}
    </div>
  );
}

export function TimelineTab({ messages, toolCalls, providerCalls, runtimeEvents }: TimelineTabProps) {
  const openToolCall = useDebugPanelStore((s) => s.openToolCall);
  const events = extractTimelineEvents(messages, toolCalls, providerCalls, runtimeEvents);

  if (events.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-muted-foreground" data-testid="timeline-empty">
        No events recorded
      </div>
    );
  }

  const items = groupIntoPipelines(events);

  return (
    <ScrollArea className="h-full" data-testid="timeline-tab">
      <div className="p-2 space-y-0.5">
        {items.map((item) => {
          if (item.type === "pipeline") {
            return (
              <PipelineSection
                key={item.startEvent.id}
                group={item}
                openToolCall={openToolCall}
              />
            );
          }
          if (item.type === "eval_group") {
            return (
              <EvalGroupSection
                key={`eval-${item.trigger}-${item.timestamp}`}
                group={item}
                openToolCall={openToolCall}
              />
            );
          }
          return (
            <EventRow
              key={item.event.id}
              event={item.event}
              openToolCall={openToolCall}
            />
          );
        })}
      </div>
    </ScrollArea>
  );
}

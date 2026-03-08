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
  ChevronDown,
  ChevronRight,
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { Message } from "@/types/session";

interface TimelineTabProps {
  readonly messages: Message[];
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

type TimelineItem =
  | { type: "event"; event: TimelineEvent }
  | PipelineGroup;

/** Group events into top-level items and collapsible pipeline sections. */
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

  return items;
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
          Pipeline
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

export function TimelineTab({ messages }: TimelineTabProps) {
  const openToolCall = useDebugPanelStore((s) => s.openToolCall);
  const events = extractTimelineEvents(messages);

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

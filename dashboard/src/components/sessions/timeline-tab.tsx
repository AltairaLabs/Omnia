"use client";

import { ScrollArea } from "@/components/ui/scroll-area";
import { Badge } from "@/components/ui/badge";
import { useDebugPanelStore } from "@/stores/debug-panel-store";
import { extractTimelineEvents, type TimelineEventKind } from "@/lib/sessions/timeline";
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

  return (
    <ScrollArea className="h-full" data-testid="timeline-tab">
      <div className="p-2 space-y-0.5">
        {events.map((event) => {
          const config = KIND_CONFIG[event.kind];
          const isClickable = event.kind === "tool_call" && event.toolCallId;

          return (
            <button
              key={event.id}
              type="button"
              className={cn(
                "flex items-center gap-3 w-full text-left px-2 py-1.5 rounded text-sm",
                "hover:bg-muted/50 transition-colors",
                isClickable && "cursor-pointer",
                !isClickable && "cursor-default"
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
        })}
      </div>
    </ScrollArea>
  );
}

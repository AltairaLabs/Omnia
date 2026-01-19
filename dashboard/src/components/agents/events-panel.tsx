"use client";

import { AlertCircle, CheckCircle2, Clock, Server, RefreshCw } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useAgentEvents, type K8sEvent } from "@/hooks";
import { cn } from "@/lib/utils";

interface EventsPanelProps {
  agentName: string;
  workspace: string;
}

/**
 * Format a timestamp to a relative time string.
 */
function formatRelativeTime(timestamp: string): string {
  const now = new Date();
  const then = new Date(timestamp);
  const diffMs = now.getTime() - then.getTime();
  const diffMins = Math.floor(diffMs / (1000 * 60));
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

  if (diffMins < 1) return "just now";
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${diffDays}d ago`;
}

/**
 * Single event row component.
 */
function EventRow({ event }: Readonly<{ event: K8sEvent }>) {
  const isWarning = event.type === "Warning";

  return (
    <div
      className={cn(
        "flex items-start gap-3 p-3 rounded-lg border",
        isWarning
          ? "border-amber-500/20 bg-amber-500/5"
          : "border-border bg-muted/30"
      )}
    >
      {/* Icon */}
      <div className="mt-0.5">
        {isWarning ? (
          <AlertCircle className="h-5 w-5 text-amber-500" />
        ) : (
          <CheckCircle2 className="h-5 w-5 text-green-500" />
        )}
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <Badge
            variant={isWarning ? "destructive" : "secondary"}
            className="text-xs"
          >
            {event.reason}
          </Badge>
          <span className="text-xs text-muted-foreground">
            {event.involvedObject.kind}/{event.involvedObject.name}
          </span>
          {event.count > 1 && (
            <Badge variant="outline" className="text-xs">
              ×{event.count}
            </Badge>
          )}
        </div>

        <p className="text-sm mt-1">{event.message}</p>

        <div className="flex items-center gap-3 mt-2 text-xs text-muted-foreground">
          <span className="flex items-center gap-1">
            <Clock className="h-3 w-3" />
            {formatRelativeTime(event.lastTimestamp)}
          </span>
          {event.source.component && (
            <span className="flex items-center gap-1">
              <Server className="h-3 w-3" />
              {event.source.component}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}

/**
 * Render events panel content based on loading/error/data state.
 */
function renderEventsPanelContent(
  isLoading: boolean,
  error: Error | null,
  events: K8sEvent[] | undefined
) {
  if (isLoading) {
    return (
      <div className="space-y-3">
        {["sk-1", "sk-2", "sk-3"].map((id) => (
          <Skeleton key={id} className="h-24 w-full" />
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        <AlertCircle className="h-8 w-8 mx-auto mb-2 opacity-50" />
        <p>Failed to load events</p>
        <p className="text-xs mt-1">{String(error)}</p>
      </div>
    );
  }

  if (!events || events.length === 0) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        <CheckCircle2 className="h-8 w-8 mx-auto mb-2 opacity-50" />
        <p>No recent events</p>
        <p className="text-xs mt-1">
          Events will appear here when they occur
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {events.map((event) => (
        <EventRow key={`${event.involvedObject.kind}-${event.involvedObject.name}-${event.reason}-${event.lastTimestamp}`} event={event} />
      ))}
    </div>
  );
}

/**
 * Events panel showing Kubernetes events for an agent.
 */
export function EventsPanel({ agentName, workspace }: Readonly<EventsPanelProps>) {
  const { data: events, isLoading, error, refetch, isFetching } = useAgentEvents(
    agentName,
    workspace
  );

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Recent Events</CardTitle>
            <CardDescription>
              Kubernetes events related to this agent
            </CardDescription>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => refetch()}
            disabled={isFetching}
          >
            <RefreshCw
              className={cn("h-4 w-4 mr-2", isFetching && "animate-spin")}
            />
            Refresh
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {renderEventsPanelContent(isLoading, error, events)}
      </CardContent>
    </Card>
  );
}

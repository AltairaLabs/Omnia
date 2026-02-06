"use client";

import { AlertCircle, CheckCircle2, RefreshCw } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
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
 * Truncate object name to show just the meaningful part.
 * e.g., "my-job-1-55bd798b86-jpmp4" -> "my-job-1-...jpmp4"
 */
function truncateObjectName(name: string, maxLen = 30): string {
  if (name.length <= maxLen) return name;
  return `${name.slice(0, maxLen - 6)}...${name.slice(-5)}`;
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
      <div className="space-y-2" data-testid="events-loading">
        {["sk-1", "sk-2", "sk-3"].map((id) => (
          <Skeleton key={id} className="h-8 w-full" />
        ))}
      </div>
    );
  }

  if (error) {
    return (
      <div className="text-center py-8 text-muted-foreground" data-testid="events-error">
        <AlertCircle className="h-8 w-8 mx-auto mb-2 opacity-50" />
        <p>Failed to load events</p>
        <p className="text-xs mt-1">{String(error)}</p>
      </div>
    );
  }

  if (!events || events.length === 0) {
    return (
      <div className="text-center py-8 text-muted-foreground" data-testid="events-empty">
        <CheckCircle2 className="h-8 w-8 mx-auto mb-2 opacity-50" />
        <p>No recent events</p>
        <p className="text-xs mt-1">
          Events will appear here when they occur
        </p>
      </div>
    );
  }

  return (
    <Table data-testid="events-table">
      <TableHeader>
        <TableRow>
          <TableHead className="w-[60px]">Type</TableHead>
          <TableHead className="w-[100px]">Reason</TableHead>
          <TableHead className="w-[180px]">Object</TableHead>
          <TableHead>Message</TableHead>
          <TableHead className="w-[80px] text-right">Age</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {events.map((event) => {
          const isWarning = event.type === "Warning";
          // Create a unique key including a hash of the message
          const messageHash = event.message.slice(0, 50).replaceAll(/[^a-zA-Z0-9]/g, "");
          return (
            <TableRow
              key={`${event.involvedObject.kind}-${event.involvedObject.name}-${event.reason}-${event.lastTimestamp}-${messageHash}`}
              className={cn(isWarning && "bg-amber-500/5")}
              data-testid="event-row"
            >
              <TableCell>
                {isWarning ? (
                  <AlertCircle className="h-4 w-4 text-amber-500" />
                ) : (
                  <CheckCircle2 className="h-4 w-4 text-green-500" />
                )}
              </TableCell>
              <TableCell>
                <Badge
                  variant={isWarning ? "destructive" : "secondary"}
                  className="text-xs font-normal"
                >
                  {event.reason}
                </Badge>
              </TableCell>
              <TableCell className="font-mono text-xs text-muted-foreground">
                <span title={`${event.involvedObject.kind}/${event.involvedObject.name}`}>
                  {event.involvedObject.kind}/{truncateObjectName(event.involvedObject.name)}
                </span>
              </TableCell>
              <TableCell className="text-sm max-w-[400px] truncate" title={event.message}>
                {event.message}
              </TableCell>
              <TableCell className="text-right text-xs text-muted-foreground whitespace-nowrap">
                {formatRelativeTime(event.lastTimestamp)}
                {event.count > 1 && (
                  <span className="ml-1 text-muted-foreground/70">×{event.count}</span>
                )}
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
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
    <Card data-testid="events-panel">
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

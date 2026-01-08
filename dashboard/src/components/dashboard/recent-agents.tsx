"use client";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { useAgents } from "@/hooks";
import { cn } from "@/lib/utils";
import type { AgentRuntimePhase } from "@/types";

const phaseColors: Record<AgentRuntimePhase, string> = {
  Running: "bg-green-500/15 text-green-700 dark:text-green-400 border-green-500/20",
  Pending: "bg-yellow-500/15 text-yellow-700 dark:text-yellow-400 border-yellow-500/20",
  Failed: "bg-red-500/15 text-red-700 dark:text-red-400 border-red-500/20",
};

export function RecentAgents() {
  const { data: agents, isLoading } = useAgents();

  if (isLoading) {
    return (
      <Card className="col-span-3">
        <CardHeader>
          <CardTitle>Recent Agents</CardTitle>
          <CardDescription>Latest agent activity</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {["sk-1", "sk-2", "sk-3", "sk-4", "sk-5"].map((id) => (
              <div key={id} className="flex items-center gap-4">
                <Skeleton className="h-10 w-10 rounded-full" />
                <div className="flex-1 space-y-2">
                  <Skeleton className="h-4 w-32" />
                  <Skeleton className="h-3 w-24" />
                </div>
                <Skeleton className="h-5 w-16" />
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    );
  }

  const recentAgents = agents
    ?.sort((a, b) => {
      const aTime = new Date(a.metadata.creationTimestamp || 0).getTime();
      const bTime = new Date(b.metadata.creationTimestamp || 0).getTime();
      return bTime - aTime;
    })
    .slice(0, 5);

  return (
    <Card className="col-span-3">
      <CardHeader>
        <CardTitle>Recent Agents</CardTitle>
        <CardDescription>Latest agent activity</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          {recentAgents?.map((agent) => (
            <div key={agent.metadata.uid} className="flex items-center gap-4">
              <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
                <span className="text-sm font-medium text-primary">
                  {agent.metadata.name.substring(0, 2).toUpperCase()}
                </span>
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium truncate">{agent.metadata.name}</p>
                <p className="text-xs text-muted-foreground">
                  {agent.metadata.namespace} &middot; {agent.spec.provider?.model || "claude-sonnet-4-20250514"}
                </p>
              </div>
              <Badge
                variant="outline"
                className={cn(
                  "text-xs",
                  agent.status?.phase && phaseColors[agent.status.phase]
                )}
              >
                {agent.status?.phase || "Unknown"}
              </Badge>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

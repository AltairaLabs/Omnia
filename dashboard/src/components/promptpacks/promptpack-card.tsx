"use client";

import Link from "next/link";
import { FileText, GitBranch, Clock } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { StatusBadge } from "@/components/agents";
import type { PromptPack } from "@/types";

interface PromptPackCardProps {
  promptPack: PromptPack;
}

function formatRelativeTime(timestamp?: string): string {
  if (!timestamp) return "-";
  const date = new Date(timestamp);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  return `${diffDays}d ago`;
}

export function PromptPackCard({ promptPack }: Readonly<PromptPackCardProps>) {
  const { metadata, spec, status } = promptPack;
  const isCanary = status?.phase === "Canary";

  return (
    <Link
      href={`/promptpacks/${metadata.name}?namespace=${metadata.namespace}`}
    >
      <Card className="hover:border-primary/50 transition-colors cursor-pointer h-full">
        <CardHeader className="pb-3">
          <div className="flex items-start justify-between gap-2">
            <div className="flex items-center gap-2 min-w-0">
              <FileText className="h-4 w-4 text-muted-foreground shrink-0" />
              <CardTitle className="text-base truncate">{metadata.name}</CardTitle>
            </div>
            <StatusBadge phase={status?.phase} />
          </div>
          <p className="text-xs text-muted-foreground">{metadata.namespace}</p>
        </CardHeader>
        <CardContent className="space-y-3">
          {/* Version info */}
          <div className="flex items-center gap-4 text-sm">
            <div className="flex items-center gap-1.5">
              <GitBranch className="h-3.5 w-3.5 text-muted-foreground" />
              <span className="text-muted-foreground">Active:</span>
              <Badge variant="secondary" className="text-xs">
                v{status?.activeVersion || spec.version}
              </Badge>
            </div>
            {isCanary && status?.canaryVersion && (
              <div className="flex items-center gap-1.5">
                <span className="text-muted-foreground">Canary:</span>
                <Badge variant="outline" className="text-xs border-violet-500/30 text-violet-600 dark:text-violet-400">
                  v{status.canaryVersion}
                </Badge>
              </div>
            )}
          </div>

          {/* Canary progress bar */}
          {isCanary && status?.canaryWeight !== undefined && (
            <div className="space-y-1.5">
              <div className="flex items-center justify-between text-xs">
                <span className="text-muted-foreground">Canary rollout</span>
                <span className="font-medium">{status.canaryWeight}%</span>
              </div>
              <Progress value={status.canaryWeight} className="h-1.5" />
            </div>
          )}

          {/* Source info */}
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span>Source:</span>
            <code className="bg-muted px-1.5 py-0.5 rounded text-xs">
              {spec.source.configMapRef?.name || "unknown"}
            </code>
          </div>

          {/* Rollout type and last updated */}
          <div className="flex items-center justify-between text-xs text-muted-foreground pt-1 border-t">
            <Badge variant="outline" className="text-xs capitalize">
              {spec.rollout.type}
            </Badge>
            <div className="flex items-center gap-1">
              <Clock className="h-3 w-3" />
              <span>{formatRelativeTime(status?.lastUpdated)}</span>
            </div>
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}

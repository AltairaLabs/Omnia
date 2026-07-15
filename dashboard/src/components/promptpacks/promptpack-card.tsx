"use client";

import Link from "next/link";
import { FileText, GitBranch, Clock } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { StatusBadge } from "@/components/agents";
import { useWorkloadTier, type WorkloadTierSummary } from "@/hooks/use-workload-tier";
import { workloadTierLabel } from "@/components/workload-graph";
import type { PromptPack } from "@/types";

interface PromptPackCardProps {
  promptPack: PromptPack;
}

export function tierSummary(wl: WorkloadTierSummary): { label: string; parts: string[] } {
  if (wl.tier === "multiagent") {
    return { label: workloadTierLabel("multiagent"), parts: [`${wl.agents} agents`, `${wl.tools} tools`] };
  }
  if (wl.tier === "workflow") {
    return { label: workloadTierLabel("workflow"), parts: [`${wl.states} states`, `${wl.tools} tools`] };
  }
  return { label: workloadTierLabel("single"), parts: [`${wl.tools} tools`] };
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
  const wl = useWorkloadTier(metadata.name, metadata.namespace);
  const { label: tierLabel, parts: tierParts } = tierSummary(wl);

  return (
    <Link
      href={`/promptpacks/${spec.packName}?namespace=${metadata.namespace}`}
    >
      <Card className="hover:border-primary/50 transition-colors cursor-pointer h-full" data-testid="promptpack-card">
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
          </div>

          {/* Workload tier */}
          {wl.tier && (
            <div className="flex items-center gap-1.5 text-xs">
              <Badge variant="outline" className="text-xs">{tierLabel}</Badge>
              <span className="text-muted-foreground">{tierParts.join(" · ")}</span>
            </div>
          )}

          {/* Source info */}
          <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span>Source:</span>
            <code className="bg-muted px-1.5 py-0.5 rounded text-xs">
              {spec.source.configMapRef?.name || "unknown"}
            </code>
          </div>

          {/* Last updated */}
          <div className="flex items-center justify-end text-xs text-muted-foreground pt-1 border-t">
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

"use client";

import { useCallback } from "react";
import Link from "next/link";
import { useQueryClient } from "@tanstack/react-query";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { StatusBadge } from "./status-badge";
import { FrameworkBadge } from "./framework-badge";
import { ScaleControl } from "./scale-control";
import { CostSparkline } from "@/components/cost";
import { getMockAgentUsage, mockCostAllocation } from "@/lib/mock-data";
import { formatCost, calculateCost } from "@/lib/pricing";
import { useDataService } from "@/lib/data";
import { useProvider } from "@/hooks";
import type { AgentRuntime } from "@/types";

interface AgentCardProps {
  agent: AgentRuntime;
}

export function AgentCard({ agent }: Readonly<AgentCardProps>) {
  const { metadata, spec, status } = agent;
  const queryClient = useQueryClient();
  const dataService = useDataService();
  const { data: provider } = useProvider(spec.providerRef?.name, metadata.namespace || "default");

  const handleScale = useCallback(async (replicas: number) => {
    await dataService.scaleAgent(metadata.namespace || "default", metadata.name, replicas);
    // Invalidate queries to refresh data
    await queryClient.invalidateQueries({ queryKey: ["agents"] });
  }, [metadata.namespace, metadata.name, queryClient, dataService]);

  // Get usage data for sparkline
  const usage = getMockAgentUsage(metadata.namespace || "default", metadata.name);
  const sparklineData = usage?.timeSeries?.map((point) => ({
    value: calculateCost(
      usage.model,
      point.inputTokens,
      point.outputTokens
    ),
  })) || [];

  // Get cost allocation for this agent
  const costData = mockCostAllocation.find(
    (c) => c.agent === metadata.name && c.namespace === metadata.namespace
  );
  const totalCost = costData?.totalCost || 0;

  // Determine sparkline color based on provider
  const providerType = provider?.spec?.type || spec.provider?.type;
  const sparklineColor = providerType === "openai" ? "#8B5CF6" : "#3B82F6";

  return (
    <Link href={`/agents/${metadata.name}?namespace=${metadata.namespace}`}>
      <Card className="transition-colors hover:bg-muted/50" data-testid="agent-card">
        <CardHeader className="pb-2">
          <div className="flex items-start justify-between">
            <div className="space-y-1">
              <CardTitle className="text-base font-medium">
                {metadata.name}
              </CardTitle>
              <CardDescription>{metadata.namespace}</CardDescription>
            </div>
            <div className="flex flex-col items-end gap-1.5">
              <StatusBadge phase={status?.phase} />
              <FrameworkBadge framework={spec.framework?.type} />
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          {/* Cost Sparkline */}
          <div className="space-y-1">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">Cost (24h)</span>
              <span className="font-medium">{formatCost(totalCost)}</span>
            </div>
            <CostSparkline data={sparklineData} color={sparklineColor} height={28} />
          </div>

          {/* Stats Grid */}
          <div className="grid grid-cols-2 gap-4 text-sm pt-1">
            <div>
              <p className="text-muted-foreground">Provider</p>
              <p className="font-medium capitalize">{providerType || "-"}</p>
            </div>
            <div>
              <p className="text-muted-foreground">Model</p>
              <p className="font-medium truncate" title={provider?.spec?.model || spec.provider?.model}>
                {(provider?.spec?.model || spec.provider?.model)?.split("-").slice(-2).join("-") || "-"}
              </p>
            </div>
            <div
              role="group"
              aria-label="Replica controls"
              onClick={(e) => e.preventDefault()}
              onKeyDown={(e) => e.stopPropagation()}
            >
              <p className="text-muted-foreground mb-1">Replicas</p>
              <ScaleControl
                currentReplicas={status?.replicas?.ready ?? 0}
                desiredReplicas={status?.replicas?.desired ?? spec.runtime?.replicas ?? 1}
                minReplicas={spec.runtime?.autoscaling?.minReplicas ?? 0}
                maxReplicas={spec.runtime?.autoscaling?.maxReplicas ?? 10}
                autoscalingEnabled={spec.runtime?.autoscaling?.enabled ?? false}
                autoscalingType={spec.runtime?.autoscaling?.type}
                onScale={handleScale}
                compact
              />
            </div>
            <div>
              <p className="text-muted-foreground">Facade</p>
              <p className="font-medium capitalize">{spec.facade?.type || "websocket"}</p>
            </div>
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}

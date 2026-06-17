/**
 * FunctionCard renders one row of the /functions catalog. Mirrors AgentCard's
 * layout — name, namespace, status + framework badges, a 24h cost sparkline
 * (same Prometheus-backed hook the agent cards use), and a stats grid — but
 * keeps the function-specific input/output field summary and click-through to
 * the function detail page.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import Link from "next/link";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { StatusBadge } from "@/components/agents/status-badge";
import { FrameworkBadge } from "@/components/agents/framework-badge";
import { CostSparkline } from "@/components/cost";
import { formatCost } from "@/lib/pricing";
import { useAgentCost } from "@/hooks/agents";
import { useProvider } from "@/hooks/resources";
import type { AgentRuntime } from "@/types";
import { getDefaultProviderRef } from "@/types/agent-runtime";

interface FunctionCardProps {
  fn: AgentRuntime;
}

/** schemaFieldCount returns the number of top-level keys under
 * schema.properties, or 0 when the schema isn't a properties-style
 * object. Used purely as a UI hint — the schema is opaque otherwise. */
export function schemaFieldCount(schema: Record<string, unknown> | undefined): number {
  if (!schema) return 0;
  const props = schema.properties;
  if (props && typeof props === "object" && !Array.isArray(props)) {
    return Object.keys(props).length;
  }
  return 0;
}

export function FunctionCard({ fn }: Readonly<FunctionCardProps>) {
  const { metadata, spec, status } = fn;
  const namespace = metadata.namespace ?? "default";
  const inputFields = schemaFieldCount(spec.inputSchema);
  const outputFields = schemaFieldCount(spec.outputSchema);
  const mcpEnabled = Boolean(spec.facade?.mcp?.enabled);

  const defaultProviderRef = getDefaultProviderRef(spec);
  const { data: provider } = useProvider(defaultProviderRef?.name, namespace);

  // Cost from the same Prometheus-backed hook the agent cards use.
  const { data: costData } = useAgentCost(namespace, metadata.name);
  const sparklineData = costData?.timeSeries || [];
  const totalCost = costData?.totalCost || 0;

  const providerType = provider?.spec?.type;
  const sparklineColor = providerType === "openai" ? "#8B5CF6" : "#3B82F6";

  return (
    <Link href={`/functions/${metadata.name}?namespace=${namespace}`}>
      <Card className="transition-colors hover:bg-muted/50" data-testid="function-card">
        <CardHeader className="pb-2">
          <div className="flex items-start justify-between gap-2">
            <div className="space-y-1">
              <CardTitle className="text-base font-medium">{metadata.name}</CardTitle>
              <CardDescription>{namespace}</CardDescription>
            </div>
            <div className="flex flex-col items-end gap-1.5">
              <StatusBadge phase={status?.phase} />
              <div className="flex items-center gap-1.5">
                {mcpEnabled && (
                  <Badge
                    variant="outline"
                    title="MCP server enabled — function callable as a typed MCP tool"
                    data-testid="mcp-badge"
                  >
                    MCP
                  </Badge>
                )}
                <FrameworkBadge framework={spec.framework?.type} />
              </div>
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
              <p className="font-medium truncate" title={provider?.spec?.model}>
                {provider?.spec?.model || "-"}
              </p>
            </div>
            <div>
              <p className="text-muted-foreground">Input</p>
              <p className="font-medium">{inputFields} field{inputFields === 1 ? "" : "s"}</p>
            </div>
            <div>
              <p className="text-muted-foreground">Output</p>
              <p className="font-medium">{outputFields} field{outputFields === 1 ? "" : "s"}</p>
            </div>
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}

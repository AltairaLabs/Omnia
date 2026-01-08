"use client";

import { useState } from "react";
import { ExternalLink, BarChart3, Loader2, AlertCircle } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  useGrafana,
  buildPanelUrl,
  buildDashboardUrl,
  GRAFANA_DASHBOARDS,
  OVERVIEW_PANELS,
  AGENT_DETAIL_PANELS,
  type GrafanaPanelOptions,
} from "@/hooks";
import { cn } from "@/lib/utils";

interface GrafanaPanelProps {
  /** Panel title */
  title: string;
  /** Panel description */
  description?: string;
  /** Dashboard UID */
  dashboardUid: string;
  /** Panel ID */
  panelId: number;
  /** Time range start */
  from?: string;
  /** Time range end */
  to?: string;
  /** Refresh interval */
  refresh?: string;
  /** Template variables */
  vars?: Record<string, string>;
  /** Panel height */
  height?: number;
  /** Additional className */
  className?: string;
  /** Fallback content when Grafana is not available */
  fallback?: React.ReactNode;
}

/**
 * Embeds a Grafana panel via iframe.
 * Shows fallback UI when Grafana is not configured.
 */
export function GrafanaPanel({
  title,
  description,
  dashboardUid,
  panelId,
  from = "now-1h",
  to = "now",
  refresh = "30s",
  vars = {},
  height = 200,
  className,
  fallback,
}: Readonly<GrafanaPanelProps>) {
  const grafana = useGrafana();
  const [isLoading, setIsLoading] = useState(true);
  const [hasError, setHasError] = useState(false);

  const panelOptions: GrafanaPanelOptions = {
    dashboardUid,
    panelId,
    from,
    to,
    refresh,
    theme: "dark",
    vars,
  };

  const panelUrl = buildPanelUrl(grafana, panelOptions);
  const dashboardUrl = buildDashboardUrl(grafana, dashboardUid, vars);

  // If Grafana is not configured, show fallback
  if (!grafana.enabled) {
    return (
      <Card className={className}>
        <CardHeader className="pb-2">
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="text-base">{title}</CardTitle>
              {description && (
                <CardDescription>{description}</CardDescription>
              )}
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {fallback || <GrafanaFallback height={height} />}
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className={className}>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="text-base">{title}</CardTitle>
            {description && (
              <CardDescription>{description}</CardDescription>
            )}
          </div>
          {dashboardUrl && (
            <Button variant="ghost" size="sm" asChild>
              <a href={dashboardUrl} target="_blank" rel="noopener noreferrer">
                <ExternalLink className="h-4 w-4" />
              </a>
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent className="p-0">
        <div className="relative" style={{ height }}>
          {isLoading && (
            <div className="absolute inset-0 flex items-center justify-center bg-muted/50">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          )}
          {hasError && (
            <div className="absolute inset-0 flex flex-col items-center justify-center bg-muted/50 gap-2">
              <AlertCircle className="h-6 w-6 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">Failed to load panel</p>
            </div>
          )}
          <iframe
            src={panelUrl || ""}
            width="100%"
            height={height}
            frameBorder="0"
            className={cn(
              "rounded-b-lg",
              isLoading && "invisible",
              hasError && "hidden"
            )}
            onLoad={() => setIsLoading(false)}
            onError={() => {
              setIsLoading(false);
              setHasError(true);
            }}
          />
        </div>
      </CardContent>
    </Card>
  );
}

/**
 * Default fallback UI when Grafana is not configured.
 */
function GrafanaFallback({ height }: Readonly<{ height: number }>) {
  return (
    <div
      className="flex flex-col items-center justify-center bg-muted/30 rounded-lg border border-dashed"
      style={{ height }}
    >
      <BarChart3 className="h-8 w-8 text-muted-foreground/50 mb-2" />
      <p className="text-sm text-muted-foreground">Grafana not configured</p>
      <p className="text-xs text-muted-foreground/70">
        Set NEXT_PUBLIC_GRAFANA_URL to enable
      </p>
    </div>
  );
}

/**
 * Pre-configured panel for agent request rate.
 */
export function AgentRequestsPanel({
  agentName,
  namespace,
  className,
}: Readonly<{
  agentName: string;
  namespace: string;
  className?: string;
}>) {
  return (
    <GrafanaPanel
      title="Requests / sec"
      description="Request rate over time"
      dashboardUid={GRAFANA_DASHBOARDS.AGENT_DETAIL}
      panelId={AGENT_DETAIL_PANELS.REQUESTS_PER_SEC}
      vars={{ agent: agentName, namespace }}
      className={className}
    />
  );
}

/**
 * Pre-configured panel for agent latency.
 */
export function AgentLatencyPanel({
  agentName,
  namespace,
  className,
}: Readonly<{
  agentName: string;
  namespace: string;
  className?: string;
}>) {
  return (
    <GrafanaPanel
      title="Latency"
      description="Response time distribution"
      dashboardUid={GRAFANA_DASHBOARDS.AGENT_DETAIL}
      panelId={AGENT_DETAIL_PANELS.P95_LATENCY}
      vars={{ agent: agentName, namespace }}
      className={className}
    />
  );
}

/**
 * Pre-configured panel for agent error rate.
 */
export function AgentErrorRatePanel({
  agentName,
  namespace,
  className,
}: Readonly<{
  agentName: string;
  namespace: string;
  className?: string;
}>) {
  return (
    <GrafanaPanel
      title="Error Rate"
      description="Errors over time"
      dashboardUid={GRAFANA_DASHBOARDS.AGENT_DETAIL}
      panelId={AGENT_DETAIL_PANELS.ERROR_RATE}
      vars={{ agent: agentName, namespace }}
      className={className}
    />
  );
}

/**
 * Pre-configured panel for token usage over time.
 */
export function TokenUsagePanel({
  agentName,
  namespace,
  className,
  height = 250,
}: Readonly<{
  agentName: string;
  namespace: string;
  className?: string;
  height?: number;
}>) {
  return (
    <GrafanaPanel
      title="Token Usage"
      description="Input and output tokens over time"
      dashboardUid={GRAFANA_DASHBOARDS.AGENT_DETAIL}
      panelId={AGENT_DETAIL_PANELS.TOKEN_USAGE}
      vars={{ agent: agentName, namespace }}
      height={height}
      className={className}
    />
  );
}

/**
 * Pre-configured panel for active connections.
 */
export function ActiveConnectionsPanel({
  agentName,
  namespace,
  className,
}: Readonly<{
  agentName: string;
  namespace: string;
  className?: string;
}>) {
  return (
    <GrafanaPanel
      title="Active Connections"
      description="Current WebSocket/gRPC connections"
      dashboardUid={GRAFANA_DASHBOARDS.AGENT_DETAIL}
      panelId={AGENT_DETAIL_PANELS.ACTIVE_CONNECTIONS}
      vars={{ agent: agentName, namespace }}
      className={className}
    />
  );
}

/**
 * Loading skeleton for Grafana panels.
 */
export function GrafanaPanelSkeleton({
  className,
  height = 200,
}: Readonly<{
  className?: string;
  height?: number;
}>) {
  return (
    <Card className={className}>
      <CardHeader className="pb-2">
        <Skeleton className="h-5 w-32" />
        <Skeleton className="h-4 w-48" />
      </CardHeader>
      <CardContent>
        <Skeleton style={{ height }} />
      </CardContent>
    </Card>
  );
}

/**
 * System-wide overview panels for the main dashboard.
 * Uses the omnia-overview dashboard panels.
 */
export function SystemOverviewPanels({ className }: Readonly<{ className?: string }>) {
  const grafana = useGrafana();

  if (!grafana.enabled) {
    return (
      <Card className={className}>
        <CardHeader>
          <CardTitle>Live Metrics</CardTitle>
          <CardDescription>Real-time system metrics via Grafana</CardDescription>
        </CardHeader>
        <CardContent>
          <GrafanaFallback height={200} />
        </CardContent>
      </Card>
    );
  }

  return (
    <div className={cn("grid md:grid-cols-2 lg:grid-cols-4 gap-4", className)}>
      <GrafanaPanel
        title="Requests/sec"
        description="Request rate across all agents"
        dashboardUid={GRAFANA_DASHBOARDS.OVERVIEW}
        panelId={OVERVIEW_PANELS.REQUESTS_PER_SEC}
        height={150}
      />
      <GrafanaPanel
        title="P95 Latency"
        description="95th percentile response time"
        dashboardUid={GRAFANA_DASHBOARDS.OVERVIEW}
        panelId={OVERVIEW_PANELS.P95_LATENCY}
        height={150}
      />
      <GrafanaPanel
        title="Cost (24h)"
        description="Estimated LLM costs"
        dashboardUid={GRAFANA_DASHBOARDS.OVERVIEW}
        panelId={OVERVIEW_PANELS.COST_24H}
        height={150}
      />
      <GrafanaPanel
        title="Tokens/min"
        description="Token throughput"
        dashboardUid={GRAFANA_DASHBOARDS.OVERVIEW}
        panelId={OVERVIEW_PANELS.TOKENS_PER_MIN}
        height={150}
      />
    </div>
  );
}

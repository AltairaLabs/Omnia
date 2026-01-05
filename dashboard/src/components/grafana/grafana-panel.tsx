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
  GRAFANA_PANELS,
} from "@/hooks";
import type { GrafanaPanelOptions } from "@/hooks";
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
}: GrafanaPanelProps) {
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
function GrafanaFallback({ height }: { height: number }) {
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
}: {
  agentName: string;
  namespace: string;
  className?: string;
}) {
  return (
    <GrafanaPanel
      title="Requests / sec"
      description="Request rate over time"
      dashboardUid={GRAFANA_DASHBOARDS.AGENT_OVERVIEW}
      panelId={GRAFANA_PANELS.REQUESTS_PER_SECOND}
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
}: {
  agentName: string;
  namespace: string;
  className?: string;
}) {
  return (
    <GrafanaPanel
      title="Latency"
      description="Response time distribution"
      dashboardUid={GRAFANA_DASHBOARDS.AGENT_OVERVIEW}
      panelId={GRAFANA_PANELS.LATENCY_HISTOGRAM}
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
}: {
  agentName: string;
  namespace: string;
  className?: string;
}) {
  return (
    <GrafanaPanel
      title="Error Rate"
      description="Errors over time"
      dashboardUid={GRAFANA_DASHBOARDS.AGENT_OVERVIEW}
      panelId={GRAFANA_PANELS.ERROR_RATE}
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
}: {
  agentName: string;
  namespace: string;
  className?: string;
  height?: number;
}) {
  return (
    <GrafanaPanel
      title="Token Usage"
      description="Input and output tokens over time"
      dashboardUid={GRAFANA_DASHBOARDS.TOKEN_USAGE}
      panelId={GRAFANA_PANELS.TOKEN_USAGE_OVER_TIME}
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
}: {
  agentName: string;
  namespace: string;
  className?: string;
}) {
  return (
    <GrafanaPanel
      title="Active Connections"
      description="Current WebSocket/gRPC connections"
      dashboardUid={GRAFANA_DASHBOARDS.AGENT_OVERVIEW}
      panelId={GRAFANA_PANELS.ACTIVE_CONNECTIONS}
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
}: {
  className?: string;
  height?: number;
}) {
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
 */
export function SystemOverviewPanels({ className }: { className?: string }) {
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
        title="Total Requests"
        description="Requests across all agents"
        dashboardUid={GRAFANA_DASHBOARDS.SYSTEM_OVERVIEW}
        panelId={GRAFANA_PANELS.TOTAL_REQUESTS}
        height={150}
      />
      <GrafanaPanel
        title="Active Agents"
        description="Running agent instances"
        dashboardUid={GRAFANA_DASHBOARDS.SYSTEM_OVERVIEW}
        panelId={GRAFANA_PANELS.TOTAL_AGENTS}
        height={150}
      />
      <GrafanaPanel
        title="Avg Latency"
        description="System-wide response time"
        dashboardUid={GRAFANA_DASHBOARDS.SYSTEM_OVERVIEW}
        panelId={GRAFANA_PANELS.SYSTEM_LATENCY}
        height={150}
      />
      <GrafanaPanel
        title="Error Rate"
        description="System-wide errors"
        dashboardUid={GRAFANA_DASHBOARDS.SYSTEM_OVERVIEW}
        panelId={GRAFANA_PANELS.SYSTEM_ERRORS}
        height={150}
      />
    </div>
  );
}

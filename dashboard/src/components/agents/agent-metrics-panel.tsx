"use client";

import { ExternalLink, Activity, Clock, AlertTriangle, Users, BarChart3 } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import {
  AreaChart,
  Area,
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from "recharts";
import { useAgentMetrics, type AgentMetric, type TokenUsagePoint } from "@/hooks/use-agent-metrics";
import { useGrafana, buildDashboardUrl, GRAFANA_DASHBOARDS } from "@/hooks/use-grafana";
import { cn } from "@/lib/utils";

interface MetricCardProps {
  title: string;
  description: string;
  metric: AgentMetric;
  icon: React.ComponentType<{ className?: string }>;
  loading?: boolean;
  color: string;
  available?: boolean;
}

/**
 * Render metric card content based on loading/availability state.
 */
function renderMetricCardContent(
  loading: boolean | undefined,
  available: boolean,
  metric: AgentMetric,
  title: string,
  color: string
) {
  const hasData = metric.series.length > 0;

  if (loading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-8 w-20" />
        <Skeleton className="h-[80px] w-full" />
      </div>
    );
  }

  if (!available) {
    return (
      <div className="flex flex-col items-center justify-center h-[108px] text-muted-foreground">
        <BarChart3 className="h-6 w-6 mb-1 opacity-50" />
        <span className="text-xs">No data</span>
      </div>
    );
  }

  return (
    <>
      <div className="text-2xl font-bold tracking-tight">
        {metric.display}
        {metric.unit && metric.unit !== "%" && (
          <span className="text-sm font-normal text-muted-foreground ml-1">
            {metric.unit}
          </span>
        )}
      </div>
      <div className="h-[80px] mt-2">
        {hasData ? (
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={metric.series}>
              <defs>
                <linearGradient id={`gradient-${title}`} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor={color} stopOpacity={0.3} />
                  <stop offset="95%" stopColor={color} stopOpacity={0} />
                </linearGradient>
              </defs>
              <Tooltip
                contentStyle={{
                  backgroundColor: "hsl(var(--card))",
                  border: "1px solid hsl(var(--border))",
                  borderRadius: "6px",
                  fontSize: "12px",
                }}
                formatter={(value) => {
                  const num = typeof value === "number" ? value : 0;
                  if (metric.unit === "%") {
                    return [`${(num * 100).toFixed(2)}%`, ""];
                  }
                  return [`${num.toFixed(2)} ${metric.unit}`, ""];
                }}
              />
              <Area
                type="monotone"
                dataKey="value"
                stroke={color}
                strokeWidth={2}
                fillOpacity={1}
                fill={`url(#gradient-${title})`}
              />
            </AreaChart>
          </ResponsiveContainer>
        ) : (
          <div className="flex items-center justify-center h-full text-xs text-muted-foreground">
            No time series data
          </div>
        )}
      </div>
    </>
  );
}

/**
 * Individual metric card with sparkline.
 */
function MetricCard({
  title,
  description,
  metric,
  icon: Icon,
  loading,
  color,
  available = true,
}: MetricCardProps) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center gap-2">
          <Icon className="h-4 w-4 text-muted-foreground" />
          <CardTitle className="text-sm font-medium">{title}</CardTitle>
        </div>
        <CardDescription className="text-xs">{description}</CardDescription>
      </CardHeader>
      <CardContent className="pb-3">
        {renderMetricCardContent(loading, available, metric, title, color)}
      </CardContent>
    </Card>
  );
}

/**
 * Render token usage chart content based on loading/availability state.
 */
function renderTokenUsageContent(
  loading: boolean | undefined,
  available: boolean | undefined,
  data: TokenUsagePoint[]
) {
  if (loading) {
    return <Skeleton className="h-[250px] w-full" />;
  }

  if (!available || data.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-[250px] text-muted-foreground">
        <BarChart3 className="h-8 w-8 mb-2 opacity-50" />
        <span className="text-sm">No token usage data</span>
      </div>
    );
  }

  return (
    <div className="h-[250px]">
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={data}>
          <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
          <XAxis
            dataKey="time"
            tick={{ fontSize: 12 }}
            tickLine={false}
            axisLine={false}
          />
          <YAxis
            tick={{ fontSize: 12 }}
            tickLine={false}
            axisLine={false}
            tickFormatter={(value) => {
              if (value >= 1000) return `${(value / 1000).toFixed(0)}K`;
              return value.toString();
            }}
          />
          <Tooltip
            contentStyle={{
              backgroundColor: "hsl(var(--card))",
              border: "1px solid hsl(var(--border))",
              borderRadius: "8px",
            }}
            formatter={(value) => {
              const num = typeof value === "number" ? value : 0;
              return [num.toLocaleString(), ""];
            }}
          />
          <Legend />
          <Line
            type="monotone"
            dataKey="input"
            name="Input"
            stroke="hsl(var(--primary))"
            strokeWidth={2}
            dot={false}
          />
          <Line
            type="monotone"
            dataKey="output"
            name="Output"
            stroke="hsl(142, 76%, 36%)"
            strokeWidth={2}
            dot={false}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

/**
 * Token usage chart showing input/output tokens over time.
 */
function TokenUsageChartPanel({
  data,
  loading,
  available,
}: {
  data: TokenUsagePoint[];
  loading?: boolean;
  available?: boolean;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Token Usage</CardTitle>
        <CardDescription>Input and output tokens over time</CardDescription>
      </CardHeader>
      <CardContent>
        {renderTokenUsageContent(loading, available, data)}
      </CardContent>
    </Card>
  );
}

interface AgentMetricsPanelProps {
  agentName: string;
  namespace: string;
  className?: string;
}

/**
 * Agent metrics panel showing real-time metrics from Prometheus.
 * Falls back to mock data in demo mode.
 */
export function AgentMetricsPanel({
  agentName,
  namespace,
  className,
}: AgentMetricsPanelProps) {
  const { data: metrics, isLoading } = useAgentMetrics(agentName, namespace);
  const grafana = useGrafana();

  const dashboardUrl = buildDashboardUrl(grafana, GRAFANA_DASHBOARDS.AGENT_DETAIL, {
    agent: agentName,
    namespace,
  });

  return (
    <div className={cn("space-y-4", className)}>
      {/* Header with Grafana link */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <h3 className="text-lg font-semibold">Agent Metrics</h3>
          {metrics.isDemo && (
            <Badge variant="secondary" className="text-xs">
              Demo Data
            </Badge>
          )}
        </div>
        {dashboardUrl && (
          <Button variant="outline" size="sm" asChild>
            <a href={dashboardUrl} target="_blank" rel="noopener noreferrer">
              <ExternalLink className="h-4 w-4 mr-2" />
              View in Grafana
            </a>
          </Button>
        )}
      </div>

      {/* Metric Cards Grid */}
      <div className="grid md:grid-cols-2 lg:grid-cols-4 gap-4">
        <MetricCard
          title="Requests/sec"
          description="Request rate"
          metric={metrics.requestsPerSec}
          icon={Activity}
          loading={isLoading}
          color="hsl(var(--primary))"
          available={metrics.available}
        />
        <MetricCard
          title="P95 Latency"
          description="95th percentile"
          metric={metrics.p95Latency}
          icon={Clock}
          loading={isLoading}
          color="hsl(142, 76%, 36%)"
          available={metrics.available}
        />
        <MetricCard
          title="Error Rate"
          description="Failed requests"
          metric={metrics.errorRate}
          icon={AlertTriangle}
          loading={isLoading}
          color="hsl(0, 84%, 60%)"
          available={metrics.available}
        />
        <MetricCard
          title="Connections"
          description="Active sessions"
          metric={metrics.activeConnections}
          icon={Users}
          loading={isLoading}
          color="hsl(262, 83%, 58%)"
          available={metrics.available}
        />
      </div>

      {/* Token Usage Chart */}
      <TokenUsageChartPanel
        data={metrics.tokenUsage}
        loading={isLoading}
        available={metrics.available}
      />
    </div>
  );
}

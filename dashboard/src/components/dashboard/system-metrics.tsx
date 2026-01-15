"use client";

import { ExternalLink, Activity, Clock, DollarSign, Coins, BarChart3 } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  AreaChart,
  Area,
  ResponsiveContainer,
  Tooltip,
} from "recharts";
import { useSystemMetrics, type SystemMetric } from "@/hooks/use-system-metrics";
import { useGrafana, buildDashboardUrl, GRAFANA_DASHBOARDS } from "@/hooks/use-grafana";
import { cn } from "@/lib/utils";

interface MetricCardProps {
  title: string;
  description: string;
  metric: SystemMetric;
  icon: React.ComponentType<{ className?: string }>;
  loading?: boolean;
  color: string;
  available?: boolean;
}

/**
 * Individual metric card with sparkline chart.
 */
/** Renders the metric card content based on loading/available state */
function MetricCardContent({
  loading,
  available,
  metric,
  title,
  color,
}: Readonly<{
  loading: boolean;
  available: boolean;
  metric: SystemMetric;
  title: string;
  color: string;
}>) {
  const hasData = metric.series.length > 0;

  if (loading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-8 w-20" />
        <Skeleton className="h-[60px] w-full" />
      </div>
    );
  }

  if (!available) {
    return (
      <div className="flex flex-col items-center justify-center h-[88px] text-muted-foreground">
        <BarChart3 className="h-6 w-6 mb-1 opacity-50" />
        <span className="text-xs">No data</span>
      </div>
    );
  }

  return (
    <>
      <div className="text-2xl font-bold tracking-tight">
        {metric.display}
        <span className="text-sm font-normal text-muted-foreground ml-1">
          {metric.unit !== "$" && metric.unit}
        </span>
      </div>
      <div className="h-[60px] mt-2">
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
                labelStyle={{ color: "hsl(var(--foreground))" }}
                formatter={(value) => {
                  const num = typeof value === "number" ? value : 0;
                  return [
                    metric.unit === "$"
                      ? `$${num.toFixed(2)}`
                      : `${num.toFixed(2)} ${metric.unit}`,
                    "",
                  ];
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

function MetricCard({
  title,
  description,
  metric,
  icon: Icon,
  loading,
  color,
  available = true,
}: Readonly<MetricCardProps>) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Icon className="h-4 w-4 text-muted-foreground" />
            <CardTitle className="text-sm font-medium">{title}</CardTitle>
          </div>
        </div>
        <CardDescription className="text-xs">{description}</CardDescription>
      </CardHeader>
      <CardContent className="pb-2">
        <MetricCardContent
          loading={loading ?? false}
          available={available}
          metric={metric}
          title={title}
          color={color}
        />
      </CardContent>
    </Card>
  );
}

/**
 * System-wide metrics panel for the dashboard overview.
 *
 * Displays real-time metrics from Prometheus with sparkline charts.
 * Falls back to empty state when Prometheus is not available.
 * Includes link to Grafana for detailed analysis.
 */
export function SystemMetrics({ className }: Readonly<{ className?: string }>) {
  const { data: metrics, isLoading } = useSystemMetrics();
  const grafana = useGrafana();

  const dashboardUrl = buildDashboardUrl(grafana, GRAFANA_DASHBOARDS.OVERVIEW);
  const available = metrics?.available ?? false;

  return (
    <div className={cn("space-y-3", className)}>
      {/* Header with Grafana link */}
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-lg font-semibold">Live Metrics</h3>
          <p className="text-sm text-muted-foreground">
            Real-time system metrics (last hour)
          </p>
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

      {/* Metrics Grid */}
      <div className="grid md:grid-cols-2 lg:grid-cols-4 gap-4">
        <MetricCard
          title="Requests/sec"
          description="Request rate across all agents"
          metric={metrics?.requestsPerSec ?? { current: 0, display: "--", series: [], unit: "req/s" }}
          icon={Activity}
          loading={isLoading}
          color="var(--primary)"
          available={available}
        />
        <MetricCard
          title="P95 Latency"
          description="95th percentile response time"
          metric={metrics?.p95Latency ?? { current: 0, display: "--", series: [], unit: "ms" }}
          icon={Clock}
          loading={isLoading}
          color="hsl(142, 76%, 36%)"
          available={available}
        />
        <MetricCard
          title="Cost (24h)"
          description="Estimated LLM costs"
          metric={metrics?.cost24h ?? { current: 0, display: "--", series: [], unit: "$" }}
          icon={DollarSign}
          loading={isLoading}
          color="hsl(38, 92%, 50%)"
          available={available}
        />
        <MetricCard
          title="Tokens/min"
          description="Token throughput"
          metric={metrics?.tokensPerMin ?? { current: 0, display: "--", series: [], unit: "tok/min" }}
          icon={Coins}
          loading={isLoading}
          color="hsl(262, 83%, 58%)"
          available={available}
        />
      </div>
    </div>
  );
}

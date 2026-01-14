"use client";

import { useMemo } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from "recharts";
import { formatCost } from "@/lib/pricing";
import { getProviderColor, getProviderDisplayName } from "@/lib/provider-utils";
import { ExternalLink } from "lucide-react";
import type { CostTimeSeriesPoint } from "@/lib/data/types";

interface CostOverTimeChartProps {
  data: CostTimeSeriesPoint[];
  title?: string;
  description?: string;
  height?: number;
  grafanaUrl?: string;
}

export function CostOverTimeChart({
  data,
  title = "Cost Over Time",
  description = "LLM costs by provider over the last 24 hours",
  height = 350,
  grafanaUrl,
}: Readonly<CostOverTimeChartProps>) {
  // Extract unique providers from data
  const providers = useMemo(() => {
    const providerSet = new Set<string>();
    for (const point of data) {
      for (const provider of Object.keys(point.byProvider)) {
        providerSet.add(provider);
      }
    }
    // Sort providers for consistent ordering
    return Array.from(providerSet).sort();
  }, [data]);

  // Format data for chart - flatten byProvider into individual keys
  const chartData = useMemo(() => {
    return data.map((point) => {
      const row: Record<string, string | number> = {
        time: new Date(point.timestamp).toLocaleTimeString([], {
          hour: "2-digit",
          minute: "2-digit",
        }),
      };
      // Add each provider's cost as a separate key
      for (const provider of providers) {
        row[getProviderDisplayName(provider)] = point.byProvider[provider] || 0;
      }
      return row;
    });
  }, [data, providers]);

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="text-base">{title}</CardTitle>
            <CardDescription>{description}</CardDescription>
          </div>
          {grafanaUrl && (
            <Button variant="ghost" size="sm" asChild>
              <a href={grafanaUrl} target="_blank" rel="noopener noreferrer">
                <ExternalLink className="h-4 w-4 mr-2" />
                View in Grafana
              </a>
            </Button>
          )}
        </div>
      </CardHeader>
      <CardContent>
        <div style={{ height }}>
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={chartData}>
              <defs>
                {providers.map((provider, index) => {
                  const color = getProviderColor(provider, index);
                  return (
                    <linearGradient
                      key={provider}
                      id={`color-${provider}`}
                      x1="0"
                      y1="0"
                      x2="0"
                      y2="1"
                    >
                      <stop offset="5%" stopColor={color} stopOpacity={0.4} />
                      <stop offset="95%" stopColor={color} stopOpacity={0} />
                    </linearGradient>
                  );
                })}
              </defs>
              <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
              <XAxis
                dataKey="time"
                tick={{ fontSize: 11 }}
                tickLine={false}
                axisLine={false}
                className="text-muted-foreground"
                interval="preserveStartEnd"
              />
              <YAxis
                tick={{ fontSize: 11 }}
                tickLine={false}
                axisLine={false}
                className="text-muted-foreground"
                tickFormatter={(value) => formatCost(value)}
              />
              <Tooltip
                contentStyle={{
                  backgroundColor: "hsl(var(--card))",
                  border: "1px solid hsl(var(--border))",
                  borderRadius: "8px",
                  fontSize: "12px",
                }}
                labelStyle={{ color: "hsl(var(--foreground))" }}
                formatter={(value) => formatCost(value as number)}
              />
              <Legend
                wrapperStyle={{ fontSize: "12px" }}
                iconType="circle"
                iconSize={8}
              />
              {providers.map((provider, index) => {
                const color = getProviderColor(provider, index);
                const displayName = getProviderDisplayName(provider);
                return (
                  <Area
                    key={provider}
                    type="monotone"
                    dataKey={displayName}
                    stackId="1"
                    stroke={color}
                    strokeWidth={2}
                    fillOpacity={1}
                    fill={`url(#color-${provider})`}
                  />
                );
              })}
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}

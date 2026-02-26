/**
 * Pass rate trend chart using Prometheus eval metrics.
 *
 * Renders a Recharts AreaChart with one area per eval metric,
 * showing how metric values change over time.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useMemo } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
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
import { useEvalPassRateTrends, type EvalTrendRange, type EvalTrendPoint } from "@/hooks";

/** Color palette for eval metric lines. */
const CHART_COLORS = [
  "#2563eb", "#16a34a", "#d97706", "#dc2626", "#7c3aed",
  "#0891b2", "#be185d", "#65a30d", "#ea580c", "#6366f1",
];

export function getEvalColor(index: number): string {
  return CHART_COLORS[index % CHART_COLORS.length];
}

const TIME_RANGE_OPTIONS: { label: string; value: EvalTrendRange }[] = [
  { label: "Last 1h", value: "1h" },
  { label: "Last 6h", value: "6h" },
  { label: "Last 24h", value: "24h" },
  { label: "Last 7d", value: "7d" },
  { label: "Last 30d", value: "30d" },
];

interface PassRateTrendChartProps {
  timeRange: EvalTrendRange;
  onTimeRangeChange: (range: EvalTrendRange) => void;
  metricNames?: string[];
  height?: number;
}

export function PassRateTrendChart({
  timeRange,
  onTimeRangeChange,
  metricNames,
  height = 350,
}: Readonly<PassRateTrendChartProps>) {
  const { data: trends, isLoading } = useEvalPassRateTrends({
    metricNames,
    timeRange,
  });

  const { chartData, seriesNames } = useMemo(() => {
    if (!trends || trends.length === 0) return { chartData: [], seriesNames: [] };

    const nameSet = new Set<string>();
    const formatted = trends.map((point: EvalTrendPoint) => {
      const row: Record<string, string | number> = {
        time: point.timestamp.toLocaleTimeString([], {
          hour: "2-digit",
          minute: "2-digit",
        }),
      };
      for (const [key, value] of Object.entries(point.values)) {
        nameSet.add(key);
        row[key] = value;
      }
      return row;
    });

    return {
      chartData: formatted,
      seriesNames: Array.from(nameSet).sort((a, b) => a.localeCompare(b)),
    };
  }, [trends]);

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <div>
            <CardTitle className="text-base">Eval Metric Trends</CardTitle>
            <CardDescription>Metric values over time from Prometheus</CardDescription>
          </div>
          <Select value={timeRange} onValueChange={(v) => onTimeRangeChange(v as EvalTrendRange)}>
            <SelectTrigger className="w-[130px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {TIME_RANGE_OPTIONS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </CardHeader>
      <CardContent>
        {isLoading && (
          <Skeleton className="w-full" style={{ height }} />
        )}
        {!isLoading && chartData.length === 0 && (
          <div className="flex items-center justify-center text-muted-foreground" style={{ height }}>
            No trend data available
          </div>
        )}
        {!isLoading && chartData.length > 0 && (
          <div style={{ height }}>
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={chartData}>
                <defs>
                  {seriesNames.map((name, index) => {
                    const color = getEvalColor(index);
                    return (
                      <linearGradient
                        key={name}
                        id={`eval-color-${name}`}
                        x1="0" y1="0" x2="0" y2="1"
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
                  domain={[0, "auto"]}
                />
                <Tooltip
                  contentStyle={{
                    backgroundColor: "hsl(var(--card))",
                    border: "1px solid hsl(var(--border))",
                    borderRadius: "8px",
                    fontSize: "12px",
                  }}
                  labelStyle={{ color: "hsl(var(--foreground))" }}
                  formatter={(value) => (value as number).toFixed(3)}
                />
                <Legend
                  wrapperStyle={{ fontSize: "12px" }}
                  iconType="circle"
                  iconSize={8}
                />
                {seriesNames.map((name, index) => {
                  const color = getEvalColor(index);
                  return (
                    <Area
                      key={name}
                      type="monotone"
                      dataKey={name}
                      stroke={color}
                      strokeWidth={2}
                      fillOpacity={1}
                      fill={`url(#eval-color-${name})`}
                    />
                  );
                })}
              </AreaChart>
            </ResponsiveContainer>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

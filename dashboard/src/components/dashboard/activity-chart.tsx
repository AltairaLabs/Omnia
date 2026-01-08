"use client";

import { BarChart3 } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import { useAgentActivity, type ActivityDataPoint } from "@/hooks";

/** Renders the appropriate chart content based on loading/data state */
function ChartContent({
  isLoading,
  available,
  isDemo,
  data,
}: {
  isLoading: boolean;
  available: boolean;
  isDemo: boolean;
  data: ActivityDataPoint[];
}) {
  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Skeleton className="h-full w-full" />
      </div>
    );
  }

  if (!available && !isDemo) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
        <BarChart3 className="h-12 w-12 mb-3 opacity-50" />
        <p className="text-sm">No activity data available</p>
        <p className="text-xs mt-1">Prometheus metrics not configured</p>
      </div>
    );
  }

  if (data.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
        <BarChart3 className="h-12 w-12 mb-3 opacity-50" />
        <p className="text-sm">No activity in the last 24 hours</p>
      </div>
    );
  }

  return (
    <ResponsiveContainer width="100%" height="100%">
      <AreaChart data={data}>
        <defs>
          <linearGradient id="requests" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="hsl(var(--primary))" stopOpacity={0.3} />
            <stop offset="95%" stopColor="hsl(var(--primary))" stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
        <XAxis
          dataKey="time"
          tick={{ fontSize: 12 }}
          tickLine={false}
          axisLine={false}
          className="text-muted-foreground"
        />
        <YAxis
          tick={{ fontSize: 12 }}
          tickLine={false}
          axisLine={false}
          className="text-muted-foreground"
        />
        <Tooltip
          contentStyle={{
            backgroundColor: "hsl(var(--card))",
            border: "1px solid hsl(var(--border))",
            borderRadius: "8px",
          }}
          labelStyle={{ color: "hsl(var(--foreground))" }}
          formatter={(value) => {
            const num = typeof value === "number" ? value : 0;
            return [num.toLocaleString(), "Requests"];
          }}
        />
        <Area
          type="monotone"
          dataKey="requests"
          stroke="hsl(var(--primary))"
          strokeWidth={2}
          fillOpacity={1}
          fill="url(#requests)"
        />
      </AreaChart>
    </ResponsiveContainer>
  );
}

export function ActivityChart() {
  const { data, available, isDemo, isLoading } = useAgentActivity();

  return (
    <Card className="col-span-4">
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Agent Activity</CardTitle>
            <CardDescription>Requests per hour across all agents</CardDescription>
          </div>
          {isDemo && (
            <Badge variant="secondary" className="text-xs">
              Demo Data
            </Badge>
          )}
        </div>
      </CardHeader>
      <CardContent>
        <div className="h-[300px]">
          <ChartContent
            isLoading={isLoading}
            available={available}
            isDemo={isDemo}
            data={data}
          />
        </div>
      </CardContent>
    </Card>
  );
}

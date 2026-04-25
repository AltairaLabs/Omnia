"use client";

import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  CartesianGrid,
} from "recharts";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import type { AggregateRow } from "@/lib/memory-analytics/types";

const RANGES = [7, 30, 90] as const;
export type RangeDays = (typeof RANGES)[number];

interface GrowthChartProps {
  rows: AggregateRow[];
  rangeDays: RangeDays;
  onRangeChange: (days: RangeDays) => void;
}

export function GrowthChart({
  rows,
  rangeDays,
  onRangeChange,
}: Readonly<GrowthChartProps>) {
  const chartData = rows.map((r) => ({ date: r.key, count: r.value }));

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle>Growth over time</CardTitle>
        <div className="flex gap-1">
          {RANGES.map((d) => (
            <Button
              key={d}
              size="sm"
              variant={d === rangeDays ? "default" : "outline"}
              onClick={() => onRangeChange(d)}
            >
              {d}d
            </Button>
          ))}
        </div>
      </CardHeader>
      <CardContent className="h-[300px]">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={chartData}>
            <CartesianGrid strokeDasharray="3 3" />
            <XAxis dataKey="date" />
            <YAxis />
            <Tooltip />
            <Area
              type="monotone"
              dataKey="count"
              stroke="hsl(217, 91%, 60%)"
              fill="hsl(217, 91%, 60%, 0.3)"
            />
          </AreaChart>
        </ResponsiveContainer>
      </CardContent>
    </Card>
  );
}

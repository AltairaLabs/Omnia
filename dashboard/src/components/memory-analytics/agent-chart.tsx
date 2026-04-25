"use client";

import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  CartesianGrid,
} from "recharts";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { AggregateRow } from "@/lib/memory-analytics/types";

const TOP_N_AGENTS = 20;

interface AgentChartProps {
  rows: AggregateRow[];
}

export function AgentChart({ rows }: Readonly<AgentChartProps>) {
  const sorted = [...rows]
    .sort((a, b) => b.value - a.value)
    .slice(0, TOP_N_AGENTS)
    .map((r) => ({ name: r.key, value: r.value }));

  return (
    <Card>
      <CardHeader>
        <CardTitle>Memory by agent</CardTitle>
      </CardHeader>
      <CardContent className="h-[400px]">
        {sorted.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No agent data yet for this workspace.
          </p>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={sorted} layout="vertical">
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis type="number" />
              <YAxis type="category" dataKey="name" width={140} />
              <Tooltip />
              <Bar dataKey="value" fill="hsl(160, 84%, 39%)" />
            </BarChart>
          </ResponsiveContainer>
        )}
      </CardContent>
    </Card>
  );
}

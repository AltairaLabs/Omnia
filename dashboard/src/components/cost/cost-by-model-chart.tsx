"use client";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Cell,
} from "recharts";
import { formatCost } from "@/lib/pricing";

interface ModelCostData {
  model: string;
  displayName: string;
  provider: string;
  cost: number;
  requests: number;
  tokens: number;
}

interface CostByModelChartProps {
  data: ModelCostData[];
  title?: string;
  description?: string;
}

// Chart colors matching globals.css
const PROVIDER_COLORS: Record<string, string> = {
  anthropic: "#3B82F6", // blue - chart-1
  openai: "#8B5CF6", // purple - chart-2
};

export function CostByModelChart({
  data,
  title = "Cost by Model",
  description = "LLM cost breakdown by model",
}: Readonly<CostByModelChartProps>) {
  // Filter out models with zero cost and sort by cost descending
  const chartData = data
    .filter((item) => item.cost > 0)
    .sort((a, b) => b.cost - a.cost);

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="h-[300px]">
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={chartData} layout="vertical" margin={{ left: 20 }}>
              <CartesianGrid strokeDasharray="3 3" className="stroke-muted" horizontal={true} vertical={false} />
              <XAxis
                type="number"
                tick={{ fontSize: 11 }}
                tickLine={false}
                axisLine={false}
                className="text-muted-foreground"
                tickFormatter={(value) => formatCost(value)}
              />
              <YAxis
                type="category"
                dataKey="displayName"
                tick={{ fontSize: 11 }}
                tickLine={false}
                axisLine={false}
                className="text-muted-foreground"
                width={100}
              />
              <Tooltip
                formatter={(value) => [formatCost(value as number), "Cost"]}
                contentStyle={{
                  backgroundColor: "hsl(var(--card))",
                  border: "1px solid hsl(var(--border))",
                  borderRadius: "8px",
                  fontSize: "12px",
                }}
                labelStyle={{ color: "hsl(var(--foreground))" }}
              />
              <Bar dataKey="cost" radius={[0, 4, 4, 0]}>
                {chartData.map((entry) => (
                  <Cell
                    key={`cell-${entry.model}`}
                    fill={PROVIDER_COLORS[entry.provider] || "#06B6D4"}
                  />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>
        {/* Legend */}
        <div className="flex justify-center gap-6 mt-4">
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded-full bg-[#3B82F6]" />
            <span className="text-xs text-muted-foreground">Anthropic</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded-full bg-[#8B5CF6]" />
            <span className="text-xs text-muted-foreground">OpenAI</span>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

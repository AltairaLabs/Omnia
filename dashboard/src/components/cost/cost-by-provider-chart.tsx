"use client";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  PieChart,
  Pie,
  Cell,
  ResponsiveContainer,
  Legend,
  Tooltip,
} from "recharts";
import { formatCost } from "@/lib/pricing";
import { getProviderColor } from "@/lib/provider-utils";

interface ProviderCostData {
  name: string;
  provider: string;
  cost: number;
  requests: number;
  tokens: number;
}

interface CostByProviderChartProps {
  data: ProviderCostData[];
  title?: string;
  description?: string;
}

export function CostByProviderChart({
  data,
  title = "Cost by Provider",
  description = "LLM cost distribution across providers",
}: Readonly<CostByProviderChartProps>) {
  const totalCost = data.reduce((sum, item) => sum + item.cost, 0);

  // Format data for pie chart
  const chartData = data.map((item) => ({
    ...item,
    percentage: totalCost > 0 ? ((item.cost / totalCost) * 100).toFixed(1) : 0,
  }));

  // Custom label renderer
  const renderLabel = (props: { name?: string; percent?: number }) => {
    const name = props.name || "";
    const percent = props.percent ? (props.percent * 100).toFixed(1) : "0";
    return `${name} (${percent}%)`;
  };

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="h-[300px]">
          <ResponsiveContainer width="100%" height="100%">
            <PieChart>
              <Pie
                data={chartData}
                cx="50%"
                cy="50%"
                labelLine={false}
                label={renderLabel}
                outerRadius={100}
                fill="#8884d8"
                dataKey="cost"
                nameKey="name"
              >
                {chartData.map((entry, index) => (
                  <Cell
                    key={`cell-${entry.provider}`}
                    fill={getProviderColor(entry.provider, index)}
                  />
                ))}
              </Pie>
              <Tooltip
                formatter={(value) => formatCost(value as number)}
                contentStyle={{
                  backgroundColor: "hsl(var(--card))",
                  border: "1px solid hsl(var(--border))",
                  borderRadius: "8px",
                  fontSize: "12px",
                }}
              />
              <Legend
                wrapperStyle={{ fontSize: "12px" }}
                formatter={(value: string) => {
                  const item = chartData.find((d) => d.name === value);
                  return `${value}: ${formatCost(item?.cost || 0)}`;
                }}
              />
            </PieChart>
          </ResponsiveContainer>
        </div>
      </CardContent>
    </Card>
  );
}

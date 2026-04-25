"use client";

import {
  PieChart,
  Pie,
  Cell,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from "recharts";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { CATEGORY_COLORS } from "@/lib/memory-analytics/colors";
import type { AggregateRow } from "@/lib/memory-analytics/types";

interface CategoryDonutProps {
  rows: AggregateRow[];
}

function colorFor(category: string): string {
  return CATEGORY_COLORS[category] ?? CATEGORY_COLORS.unknown;
}

export function CategoryDonut({ rows }: Readonly<CategoryDonutProps>) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Memory by category</CardTitle>
      </CardHeader>
      <CardContent className="h-[300px]">
        {rows.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No memory data yet for this workspace.
          </p>
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <PieChart>
              <Pie
                data={rows}
                dataKey="value"
                nameKey="key"
                innerRadius={60}
                outerRadius={100}
                paddingAngle={2}
              >
                {rows.map((row) => (
                  <Cell key={row.key} fill={colorFor(row.key)} />
                ))}
              </Pie>
              <Tooltip />
              <Legend />
            </PieChart>
          </ResponsiveContainer>
        )}
      </CardContent>
    </Card>
  );
}

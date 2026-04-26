"use client";

import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { ConsentStats } from "@/lib/memory-analytics/types";
import { CATEGORY_COLORS } from "@/lib/memory-analytics/colors";

interface PrivacyPostureProps {
  stats: ConsentStats;
}

function optOutPercent(stats: ConsentStats): number {
  if (stats.totalUsers === 0) return 0;
  return (stats.optedOutAll / stats.totalUsers) * 100;
}

function grantRows(stats: ConsentStats) {
  // Backend may omit grantsByCategory when there are no consent grants
  // (or send null on certain edge cases). Default to {} so Object.entries
  // doesn't throw.
  const grants = stats.grantsByCategory ?? {};
  return Object.entries(grants)
    .map(([key, value]) => ({ name: key, value }))
    .sort((a, b) => b.value - a.value);
}

export function PrivacyPosture({ stats }: Readonly<PrivacyPostureProps>) {
  const optOut = optOutPercent(stats);
  const grants = grantRows(stats);

  return (
    <Card>
      <CardHeader>
        <CardTitle>Privacy posture</CardTitle>
      </CardHeader>
      <CardContent className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div>
          <p className="text-sm font-medium mb-2">Consent grants by category</p>
          <div className="h-[200px]">
            {grants.length === 0 ? (
              <p className="text-xs text-muted-foreground">
                No consent data yet.
              </p>
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={grants}>
                  <XAxis dataKey="name" tick={{ fontSize: 10 }} />
                  <YAxis />
                  <Tooltip />
                  <Bar
                    dataKey="value"
                    fill={CATEGORY_COLORS["memory:context"]}
                  />
                </BarChart>
              </ResponsiveContainer>
            )}
          </div>
        </div>
        <div>
          <p className="text-sm font-medium mb-2">Opt-out rate</p>
          <div className="text-3xl font-bold">{optOut.toFixed(1)}%</div>
          <p className="text-xs text-muted-foreground">
            {stats.optedOutAll} of {stats.totalUsers} users
          </p>
        </div>
        <div>
          <p className="text-sm font-medium mb-2">Redaction activity</p>
          <div className="text-3xl font-bold">0</div>
          <p className="text-xs text-muted-foreground">
            Audit integration pending
          </p>
        </div>
      </CardContent>
    </Card>
  );
}

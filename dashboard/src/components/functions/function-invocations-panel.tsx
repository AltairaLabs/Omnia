/**
 * FunctionInvocationsPanel renders the per-function audit history.
 *
 * Two horizontal sparklines (latency + cost) over the loaded window,
 * then a chronological table of the most recent rows. Time window
 * is controlled by the parent via the `windowMs` prop so the panel
 * stays stateless and reusable.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useMemo } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Sparkline } from "@/components/ui/sparkline";
import { Skeleton } from "@/components/ui/skeleton";
import { useFunctionInvocations } from "@/hooks/use-function-invocations";
import { formatCost } from "@/lib/pricing";
import type {
  FunctionInvocation,
  FunctionInvocationStatus,
} from "@/lib/data/function-invocations-service";

interface FunctionInvocationsPanelProps {
  workspace: string;
  functionName: string;
  /** Window size in milliseconds; rows older than (now - windowMs)
   * are not requested. Caller picks the preset (24h / 7d / 30d). */
  windowMs: number;
  /** Cap rows returned. session-api clamps server-side to 1000. */
  limit?: number;
}

const STATUS_VARIANT: Record<FunctionInvocationStatus, "default" | "secondary" | "destructive" | "outline"> = {
  success: "default",
  input_invalid: "outline",
  output_invalid: "destructive",
  runtime_error: "destructive",
};

const STATUS_LABEL: Record<FunctionInvocationStatus, string> = {
  success: "Success",
  input_invalid: "Input Invalid",
  output_invalid: "Output Invalid",
  runtime_error: "Runtime Error",
};

/** statsFromRows computes the summary that headlines the panel. The
 * sparkline arrays are in chronological order (oldest first) so the
 * eye reads left → right as time → present. */
function statsFromRows(rows: FunctionInvocation[]) {
  if (rows.length === 0) {
    return {
      count: 0,
      avgLatencyMs: 0,
      totalCost: 0,
      latencySeries: [] as Array<{ value: number }>,
      costSeries: [] as Array<{ value: number }>,
    };
  }
  // Session-api returns DESC by created_at; reverse so the sparkline
  // reads chronologically rather than as a recency cliff.
  const chrono = [...rows].reverse();
  const latencySeries = chrono.map((r) => ({ value: r.durationMs }));
  const costSeries = chrono.map((r) => ({ value: r.costUsd }));
  const totalCost = rows.reduce((acc, r) => acc + r.costUsd, 0);
  const avgLatencyMs = rows.reduce((acc, r) => acc + r.durationMs, 0) / rows.length;
  return {
    count: rows.length,
    avgLatencyMs,
    totalCost,
    latencySeries,
    costSeries,
  };
}

function formatLatency(ms: number): string {
  if (ms >= 1000) return `${(ms / 1000).toFixed(2)}s`;
  return `${Math.round(ms)}ms`;
}

function formatTimestamp(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export function FunctionInvocationsPanel({
  workspace,
  functionName,
  windowMs,
  limit = 100,
}: Readonly<FunctionInvocationsPanelProps>) {
  // Recompute on every render so the window slides forward in time
  // without needing a refetch trigger; the query key only cares about
  // the resolved ISO bounds.
  const now = new Date();
  const fromIso = useMemo(
    () => new Date(now.getTime() - windowMs).toISOString(),
    // `now` is intentionally NOT in deps — we want a fresh bound each
    // render but we still want the query key to be stable enough that
    // re-renders inside the same minute don't trigger a refetch.
    // Truncate to the minute so the key churns once per minute, not
    // on every paint.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [Math.floor(now.getTime() / 60_000), windowMs],
  );

  const { data, isLoading, error } = useFunctionInvocations({
    workspace,
    functionName,
    fromIso,
    limit,
  });

  const rows = useMemo(() => data ?? [], [data]);
  const stats = useMemo(() => statsFromRows(rows), [rows]);

  if (isLoading) {
    return (
      <Card data-testid="function-invocations-panel-loading">
        <CardHeader>
          <CardTitle>Recent invocations</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
        </CardContent>
      </Card>
    );
  }

  if (error) {
    return (
      <Card data-testid="function-invocations-panel-error">
        <CardHeader>
          <CardTitle>Recent invocations</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-destructive">
            Failed to load invocations: {error instanceof Error ? error.message : String(error)}
          </p>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card data-testid="function-invocations-panel">
      <CardHeader>
        <CardTitle>Recent invocations</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-3 gap-4">
          <Stat label="Invocations" value={String(stats.count)} />
          <Stat
            label="Avg latency"
            value={formatLatency(stats.avgLatencyMs)}
            sparkline={
              <Sparkline data={stats.latencySeries} strokeColor="#3B82F6" width={120} height={32} />
            }
          />
          <Stat
            label="Total cost"
            value={formatCost(stats.totalCost)}
            sparkline={
              <Sparkline data={stats.costSeries} strokeColor="#10B981" width={120} height={32} />
            }
          />
        </div>

        {rows.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            No invocations recorded in this window.
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm" data-testid="function-invocations-table">
              <thead className="text-left text-xs uppercase text-muted-foreground">
                <tr>
                  <th className="py-2 pr-4">When</th>
                  <th className="py-2 pr-4">Status</th>
                  <th className="py-2 pr-4 text-right">Latency</th>
                  <th className="py-2 pr-4 text-right">Cost</th>
                  <th className="py-2 pr-4">Trace</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.id} className="border-t" data-testid="function-invocations-row">
                    <td className="py-2 pr-4 font-mono text-xs">
                      {formatTimestamp(row.createdAt)}
                    </td>
                    <td className="py-2 pr-4">
                      <Badge variant={STATUS_VARIANT[row.status] ?? "outline"}>
                        {STATUS_LABEL[row.status] ?? row.status}
                      </Badge>
                    </td>
                    <td className="py-2 pr-4 text-right">{formatLatency(row.durationMs)}</td>
                    <td className="py-2 pr-4 text-right">{formatCost(row.costUsd)}</td>
                    <td className="py-2 pr-4 font-mono text-xs text-muted-foreground">
                      {row.traceId ? row.traceId.slice(0, 12) : "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

interface StatProps {
  label: string;
  value: string;
  sparkline?: React.ReactNode;
}

function Stat({ label, value, sparkline }: Readonly<StatProps>) {
  return (
    <div>
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="text-lg font-semibold">{value}</p>
      {sparkline ? <div className="mt-1">{sparkline}</div> : null}
    </div>
  );
}

/**
 * FunctionSessionsPanel renders the per-function invocation history
 * sourced from the standard `sessions` data path. Function-mode
 * invocations record as ordinary sessions tagged "function" (see the
 * Functions-as-sessions rework), so this panel reuses the existing
 * useSessions hook + workspace-scoped session-api proxy that already
 * powers /sessions.
 *
 * Each row links into the standard session detail view so operators
 * can drill from a Loki log line → session id → full audit trail.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import Link from "next/link";
import { ArrowUpRight } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useSessions } from "@/hooks/sessions";
import { formatCost } from "@/lib/pricing";
import type { SessionSummary } from "@/types/session";

interface FunctionSessionsPanelProps {
  functionName: string;
  limit?: number;
}

const STATUS_VARIANT: Record<
  SessionSummary["status"],
  "default" | "secondary" | "destructive" | "outline"
> = {
  active: "secondary",
  completed: "default",
  error: "destructive",
  expired: "outline",
};

const STATUS_LABEL: Record<SessionSummary["status"], string> = {
  active: "Active",
  completed: "Completed",
  error: "Error",
  expired: "Expired",
};

/** durationMs computes the runtime of a session from its start/end
 * timestamps. Active rows (no endedAt) show "—" instead. */
function durationMs(s: SessionSummary): number | null {
  if (!s.endedAt) return null;
  const start = Date.parse(s.startedAt);
  const end = Date.parse(s.endedAt);
  if (Number.isNaN(start) || Number.isNaN(end)) return null;
  return Math.max(0, end - start);
}

function formatLatency(ms: number | null): string {
  if (ms === null) return "—";
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

export function FunctionSessionsPanel({
  functionName,
  limit = 50,
}: Readonly<FunctionSessionsPanelProps>) {
  const { data, isLoading, error } = useSessions({
    agent: functionName,
    limit,
  });

  if (isLoading) {
    return (
      <Card data-testid="function-sessions-panel-loading">
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
      <Card data-testid="function-sessions-panel-error">
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

  const sessions = data?.sessions ?? [];

  return (
    <Card data-testid="function-sessions-panel">
      <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
        <CardTitle>Recent invocations</CardTitle>
        {sessions.length > 0 && (
          <Link
            href={`/sessions?agent=${encodeURIComponent(functionName)}`}
            className="flex items-center gap-0.5 text-sm text-muted-foreground hover:text-foreground hover:underline"
          >
            View all
            <ArrowUpRight className="h-3.5 w-3.5" />
          </Link>
        )}
      </CardHeader>
      <CardContent>
        {sessions.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            No invocations recorded yet.
          </p>
        ) : (
          <Table data-testid="function-sessions-table">
            <TableHeader>
              <TableRow>
                <TableHead>When</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Latency</TableHead>
                <TableHead className="text-right">Cost</TableHead>
                <TableHead>Session</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sessions.map((s) => (
                <TableRow key={s.id} data-testid="function-sessions-row">
                  <TableCell className="font-mono text-xs">{formatTimestamp(s.startedAt)}</TableCell>
                  <TableCell>
                    <Badge variant={STATUS_VARIANT[s.status] ?? "outline"}>
                      {STATUS_LABEL[s.status] ?? s.status}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-right">{formatLatency(durationMs(s))}</TableCell>
                  <TableCell className="text-right">{formatCost(s.estimatedCost ?? 0)}</TableCell>
                  <TableCell>
                    <Link
                      href={`/sessions/${s.id}`}
                      className="font-mono text-xs text-muted-foreground hover:text-foreground hover:underline"
                    >
                      {s.id.slice(0, 8)}…
                    </Link>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}

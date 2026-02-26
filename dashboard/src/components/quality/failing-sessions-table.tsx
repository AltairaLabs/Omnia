/**
 * Failing sessions drill-down table.
 *
 * Shown when user selects a metric in the breakdown panel. Fetches
 * recent eval failures from session-api filtered by eval type.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import Link from "next/link";
import { Card, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { AlertCircle, Bot, ExternalLink, XCircle } from "lucide-react";
import { useRecentEvalFailures } from "@/hooks";
import { formatDistanceToNow } from "date-fns";

interface FailingSessionsTableProps {
  evalType?: string;
  agentName?: string;
  limit?: number;
}

export function FailingSessionsTable({
  evalType,
  agentName,
  limit = 10,
}: Readonly<FailingSessionsTableProps>) {
  const { data, isLoading, error } = useRecentEvalFailures({
    evalType,
    agentName,
    passed: false,
    limit,
  });

  const failures = data?.evalResults ?? [];
  const displayName = evalType
    ? evalType.replaceAll(/^omnia_eval_/g, "").replaceAll("_", " ")
    : "All Types";

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Failing Sessions</CardTitle>
        <CardDescription>
          Recent failures for: {displayName}
        </CardDescription>
      </CardHeader>
      {error && (
        <div className="px-6 pb-4">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>Failed to load failing sessions</AlertDescription>
          </Alert>
        </div>
      )}
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Eval ID</TableHead>
            <TableHead>Agent</TableHead>
            <TableHead>Pack</TableHead>
            <TableHead>Time</TableHead>
            <TableHead></TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading && (
            <>
              <SkeletonRow />
              <SkeletonRow />
              <SkeletonRow />
            </>
          )}
          {!isLoading && failures.length === 0 && (
            <TableRow>
              <TableCell colSpan={5} className="text-center py-8 text-muted-foreground">
                No recent failures
              </TableCell>
            </TableRow>
          )}
          {!isLoading && failures.map((result) => (
            <TableRow key={result.id}>
              <TableCell className="font-mono text-sm">
                <div className="flex items-center gap-2">
                  <XCircle className="h-4 w-4 text-red-500 shrink-0" />
                  {result.evalId}
                </div>
              </TableCell>
              <TableCell>
                <div className="flex items-center gap-2">
                  <Bot className="h-4 w-4 text-muted-foreground shrink-0" />
                  {result.agentName}
                </div>
              </TableCell>
              <TableCell>
                <Badge variant="outline">{result.promptpackName}</Badge>
              </TableCell>
              <TableCell className="text-muted-foreground">
                {formatDistanceToNow(new Date(result.createdAt), { addSuffix: true })}
              </TableCell>
              <TableCell>
                <Link href={`/sessions/${result.sessionId}`} className="text-primary hover:underline">
                  <ExternalLink className="h-4 w-4" />
                </Link>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  );
}

function SkeletonRow() {
  return (
    <TableRow>
      <TableCell><Skeleton className="h-4 w-24" /></TableCell>
      <TableCell><Skeleton className="h-4 w-20" /></TableCell>
      <TableCell><Skeleton className="h-4 w-16" /></TableCell>
      <TableCell><Skeleton className="h-4 w-24" /></TableCell>
      <TableCell><Skeleton className="h-4 w-4" /></TableCell>
    </TableRow>
  );
}

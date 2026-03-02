/**
 * Assertion type breakdown panel.
 *
 * Discovers all omnia_eval_* metrics from Prometheus and displays them
 * grouped by metric name with current values. Clickable rows set the
 * active metric for drill-down in other panels.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { Card, CardHeader, CardTitle } from "@/components/ui/card";
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
import { useEvalMetrics, type EvalMetricInfo } from "@/hooks";
import type { PrometheusMetricType } from "@/lib/prometheus";
import type { EvalFilter } from "@/lib/prometheus-queries";

/** Check if a metric type represents a 0-1 score (gauge or boolean). */
function isScoreType(metricType: PrometheusMetricType): boolean {
  return metricType === "gauge" || metricType === "unknown";
}

/** Format a metric name for display by stripping the prefix and converting underscores. */
export function formatMetricName(name: string): string {
  return name.replaceAll(/^omnia_eval_/g, "").replaceAll("_", " ");
}

/** Get a color-coded badge variant based on metric value (0-1 scale). */
export function getMetricVariant(value: number, metricType: PrometheusMetricType = "gauge"): "default" | "secondary" | "destructive" | "outline" {
  if (!isScoreType(metricType)) return "outline";
  if (value >= 0.9) return "default";
  if (value >= 0.7) return "secondary";
  return "destructive";
}

/** Get a text color class based on metric value (0-1 scale). */
export function getMetricColor(value: number, metricType: PrometheusMetricType = "gauge"): string {
  if (!isScoreType(metricType)) return "text-muted-foreground";
  if (value >= 0.9) return "text-green-600 dark:text-green-400";
  if (value >= 0.7) return "text-yellow-600 dark:text-yellow-400";
  return "text-red-600 dark:text-red-400";
}

/** Format metric value for display based on its type. */
export function formatMetricValue(value: number, metricType: PrometheusMetricType): string {
  if (metricType === "counter") return Math.round(value).toLocaleString();
  if (metricType === "histogram") return `${value.toFixed(3)}s`;
  return value.toFixed(3);
}

/** Get status label for a metric based on its type and value. */
function getStatusLabel(value: number, metricType: PrometheusMetricType): string {
  if (!isScoreType(metricType)) return metricType;
  if (value >= 0.9) return "Passing";
  if (value >= 0.7) return "Warning";
  return "Failing";
}

interface AssertionTypeBreakdownProps {
  activeMetric?: string;
  onSelectMetric?: (metricName: string) => void;
  filter?: EvalFilter;
}

export function AssertionTypeBreakdown({
  activeMetric,
  onSelectMetric,
  filter,
}: Readonly<AssertionTypeBreakdownProps>) {
  const { data: metrics, isLoading, error } = useEvalMetrics(filter);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Eval Metrics Breakdown</CardTitle>
      </CardHeader>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Metric</TableHead>
            <TableHead>Current Value</TableHead>
            <TableHead className="text-right">Status</TableHead>
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
          {error && (
            <TableRow>
              <TableCell colSpan={3} className="text-center py-8 text-muted-foreground">
                Unable to load eval metrics from Prometheus
              </TableCell>
            </TableRow>
          )}
          {!isLoading && !error && metrics?.length === 0 && (
            <TableRow>
              <TableCell colSpan={3} className="text-center py-8 text-muted-foreground">
                No eval metrics found
              </TableCell>
            </TableRow>
          )}
          {!isLoading && metrics?.map((metric) => (
            <MetricRow
              key={metric.name}
              metric={metric}
              isActive={activeMetric === metric.name}
              onSelect={onSelectMetric}
            />
          ))}
        </TableBody>
      </Table>
    </Card>
  );
}

function MetricRow({
  metric,
  isActive,
  onSelect,
}: Readonly<{
  metric: EvalMetricInfo;
  isActive: boolean;
  onSelect?: (name: string) => void;
}>) {
  return (
    <TableRow
      className={`cursor-pointer ${isActive ? "bg-muted" : ""}`}
      onClick={() => onSelect?.(metric.name)}
    >
      <TableCell className="font-mono text-sm">
        {formatMetricName(metric.name)}
      </TableCell>
      <TableCell>
        <span className={getMetricColor(metric.value, metric.metricType)}>
          {formatMetricValue(metric.value, metric.metricType)}
        </span>
      </TableCell>
      <TableCell className="text-right">
        <Badge variant={getMetricVariant(metric.value, metric.metricType)}>
          {getStatusLabel(metric.value, metric.metricType)}
        </Badge>
      </TableCell>
    </TableRow>
  );
}

function SkeletonRow() {
  return (
    <TableRow>
      <TableCell><Skeleton className="h-4 w-32" /></TableCell>
      <TableCell><Skeleton className="h-4 w-16" /></TableCell>
      <TableCell className="text-right"><Skeleton className="h-4 w-16 ml-auto" /></TableCell>
    </TableRow>
  );
}

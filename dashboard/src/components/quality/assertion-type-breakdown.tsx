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

/** Format a metric name for display by stripping the prefix and converting underscores. */
export function formatMetricName(name: string): string {
  return name.replaceAll(/^omnia_eval_/g, "").replaceAll("_", " ");
}

/** Get a color-coded badge variant based on metric type. */
export function getMetricVariant(_value: number, metricType: PrometheusMetricType = "gauge"): "default" | "outline" {
  if (metricType === "gauge" || metricType === "unknown") return "default";
  return "outline";
}

/** Get a text color class based on metric type. */
export function getMetricColor(_value: number, metricType: PrometheusMetricType = "gauge"): string {
  if (metricType === "gauge" || metricType === "unknown") return "text-foreground";
  return "text-muted-foreground";
}

/** Format metric value for display based on its type. */
export function formatMetricValue(value: number, metricType: PrometheusMetricType): string {
  if (metricType === "counter") return Math.round(value).toLocaleString();
  if (metricType === "histogram") return `${value.toFixed(3)}s`;
  return value.toFixed(3);
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
            <TableHead className="text-right">Type</TableHead>
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
        <Badge variant="outline">
          {metric.metricType}
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

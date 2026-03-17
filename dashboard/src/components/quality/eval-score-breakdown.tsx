/**
 * Eval score breakdown panel.
 *
 * Discovers all omnia_eval_* metrics from Prometheus and displays them
 * as compact cards sorted worst-first (ascending by value):
 * - gauge (0–1): sparkline + progress bar + percentage
 * - boolean (0 or 1): step sparkline + pass/fail badge
 * - counter: sparkline + current count
 * - histogram: sparkline + current duration
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useMemo } from "react";
import { Card, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";
import { Sparkline } from "@/components/ui/sparkline";
import { useEvalMetrics, type EvalMetricInfo } from "@/hooks/sessions";
import type { EvalFilter } from "@/lib/prometheus-queries";

/** Format a metric name for display by stripping the prefix and converting underscores. */
export function formatMetricName(name: string): string {
  return name.replaceAll(/^omnia_eval_/g, "").replaceAll("_", " ");
}

/** Get a color class for gauge values based on threshold. */
export function getGaugeColor(value: number): string {
  if (value >= 0.9) return "text-green-600 dark:text-green-400";
  if (value >= 0.7) return "text-yellow-600 dark:text-yellow-400";
  return "text-red-600 dark:text-red-400";
}

/** Get a CSS class for Progress indicator color based on gauge value. */
export function getGaugeBarClass(value: number): string {
  if (value >= 0.9) return "[&>[data-slot=progress-indicator]]:bg-green-500";
  if (value >= 0.7) return "[&>[data-slot=progress-indicator]]:bg-yellow-500";
  return "[&>[data-slot=progress-indicator]]:bg-red-500";
}

/** Get sparkline stroke color based on gauge value. */
function getSparklineColor(value: number): string {
  if (value >= 0.9) return "rgb(34, 197, 94)";
  if (value >= 0.7) return "rgb(234, 179, 8)";
  return "rgb(239, 68, 68)";
}

interface EvalScoreBreakdownProps {
  activeMetric?: string;
  onSelectMetric?: (metricName: string) => void;
  filter?: EvalFilter;
}

export function EvalScoreBreakdown({
  activeMetric,
  onSelectMetric,
  filter,
}: Readonly<EvalScoreBreakdownProps>) {
  const { data: metrics, isLoading, error } = useEvalMetrics(filter);

  const sorted = useMemo(() => {
    if (!metrics) return [];
    return [...metrics].sort((a, b) => a.value - b.value);
  }, [metrics]);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Eval Breakdown</CardTitle>
      </CardHeader>
      <div className="px-4 pb-4 space-y-2">
        {isLoading && (
          <>
            <Skeleton className="h-16 w-full rounded-md" />
            <Skeleton className="h-16 w-full rounded-md" />
            <Skeleton className="h-16 w-full rounded-md" />
          </>
        )}
        {error && (
          <div className="text-center py-8 text-muted-foreground text-sm">
            Unable to load eval metrics from Prometheus
          </div>
        )}
        {!isLoading && !error && sorted.length === 0 && (
          <div className="text-center py-8 text-muted-foreground text-sm">
            No eval metrics found
          </div>
        )}
        {!isLoading && sorted.map((metric) => (
          <MetricCard
            key={metric.name}
            metric={metric}
            isActive={activeMetric === metric.name}
            onSelect={onSelectMetric}
          />
        ))}
      </div>
    </Card>
  );
}

function MetricCard({
  metric,
  isActive,
  onSelect,
}: Readonly<{
  metric: EvalMetricInfo;
  isActive: boolean;
  onSelect?: (name: string) => void;
}>) {
  return (
    <div
      role="button"
      tabIndex={0}
      className={`flex items-center gap-3 p-3 rounded-md border cursor-pointer transition-colors ${
        isActive ? "bg-muted border-primary" : "hover:bg-muted/50"
      }`}
      onClick={() => onSelect?.(metric.name)}
      onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") onSelect?.(metric.name); }}
    >
      <div className="flex-1 min-w-0">
        <div className="font-mono text-sm truncate">{formatMetricName(metric.name)}</div>
        <MetricValue value={metric.value} metricType={metric.metricType} />
      </div>
      <div className="flex-shrink-0">
        <MetricSparkline metric={metric} />
      </div>
    </div>
  );
}

/** Renders the current value in a type-appropriate format. */
function MetricValue({ value, metricType }: Readonly<{ value: number; metricType: string }>) {
  if (metricType === "gauge") {
    const pct = Math.round(value * 100);
    return (
      <div className="flex items-center gap-2 mt-1" data-testid="gauge-display">
        <Progress value={pct} className={`h-1.5 w-20 ${getGaugeBarClass(value)}`} />
        <span className={`text-xs font-medium ${getGaugeColor(value)}`}>{pct}%</span>
      </div>
    );
  }

  if (metricType === "counter") {
    return <span className="text-xs font-mono text-muted-foreground mt-1 block" data-testid="counter-display">{Math.round(value).toLocaleString()}</span>;
  }

  if (metricType === "histogram") {
    return <span className="text-xs font-mono text-muted-foreground mt-1 block" data-testid="histogram-display">{value.toFixed(3)}s</span>;
  }

  return <span className="text-xs mt-1 block">{value.toFixed(3)}</span>;
}

/** Renders an inline sparkline with color appropriate to the metric type. */
function MetricSparkline({ metric }: Readonly<{ metric: EvalMetricInfo }>) {
  if (!metric.sparkline || metric.sparkline.length < 2) {
    return <div className="w-[80px] h-[24px]" />;
  }

  const color = metric.metricType === "gauge"
    ? getSparklineColor(metric.value)
    : "rgb(148, 163, 184)";

  return (
    <Sparkline
      data={metric.sparkline}
      width={80}
      height={24}
      strokeColor={color}
      fillColor={color}
      strokeWidth={1.5}
    />
  );
}

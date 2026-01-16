"use client";

import { useMemo } from "react";
import { cn } from "@/lib/utils";

interface SparklineProps {
  data: Array<{ value: number }>;
  width?: number;
  height?: number;
  className?: string;
  strokeColor?: string;
  fillColor?: string;
  strokeWidth?: number;
  showArea?: boolean;
}

/**
 * Simple SVG sparkline component for mini charts.
 */
export function Sparkline({
  data,
  width = 100,
  height = 24,
  className,
  strokeColor = "currentColor",
  fillColor,
  strokeWidth = 1.5,
  showArea = true,
}: SparklineProps) {
  const path = useMemo(() => {
    if (!data || data.length < 2) return { line: "", area: "" };

    const values = data.map((d) => d.value);
    const min = Math.min(...values);
    const max = Math.max(...values);
    const range = max - min || 1;

    // Add padding at top and bottom
    const padding = 2;
    const chartHeight = height - padding * 2;
    const chartWidth = width;

    // Calculate points
    const points = values.map((value, index) => {
      const x = (index / (values.length - 1)) * chartWidth;
      const y = padding + chartHeight - ((value - min) / range) * chartHeight;
      return { x, y };
    });

    // Build SVG path for line
    const linePath = points
      .map((point, index) => {
        const command = index === 0 ? "M" : "L";
        return `${command}${point.x.toFixed(2)},${point.y.toFixed(2)}`;
      })
      .join(" ");

    // Build SVG path for area (closed polygon)
    const areaPath =
      linePath +
      ` L${chartWidth.toFixed(2)},${(height - padding).toFixed(2)}` +
      ` L0,${(height - padding).toFixed(2)} Z`;

    return { line: linePath, area: areaPath };
  }, [data, width, height]);

  if (!data || data.length < 2) {
    return (
      <div
        className={cn("inline-flex items-center justify-center text-muted-foreground", className)}
        style={{ width, height }}
      >
        <span className="text-xs">--</span>
      </div>
    );
  }

  return (
    <svg
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      className={cn("overflow-visible", className)}
    >
      {showArea && fillColor && (
        <path
          d={path.area}
          fill={fillColor}
          opacity={0.2}
        />
      )}
      <path
        d={path.line}
        fill="none"
        stroke={strokeColor}
        strokeWidth={strokeWidth}
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

/**
 * Metric card with sparkline visualization.
 */
interface MetricSparklineCardProps {
  title: string;
  value: string | number;
  unit?: string;
  data: Array<{ value: number }>;
  color?: "default" | "green" | "blue" | "orange" | "red";
  trend?: "up" | "down" | "neutral";
  className?: string;
}

const colorMap = {
  default: {
    stroke: "rgb(var(--foreground))",
    fill: "rgb(var(--foreground))",
    text: "text-foreground",
  },
  green: {
    stroke: "rgb(34, 197, 94)",
    fill: "rgb(34, 197, 94)",
    text: "text-green-600 dark:text-green-400",
  },
  blue: {
    stroke: "rgb(59, 130, 246)",
    fill: "rgb(59, 130, 246)",
    text: "text-blue-600 dark:text-blue-400",
  },
  orange: {
    stroke: "rgb(249, 115, 22)",
    fill: "rgb(249, 115, 22)",
    text: "text-orange-600 dark:text-orange-400",
  },
  red: {
    stroke: "rgb(239, 68, 68)",
    fill: "rgb(239, 68, 68)",
    text: "text-red-600 dark:text-red-400",
  },
};

export function MetricSparklineCard({
  title,
  value,
  unit,
  data,
  color = "default",
  className,
}: MetricSparklineCardProps) {
  const colors = colorMap[color];

  return (
    <div className={cn("p-4 rounded-lg border bg-card", className)}>
      <div className="flex items-center justify-between mb-2">
        <span className="text-sm text-muted-foreground">{title}</span>
        <Sparkline
          data={data}
          width={60}
          height={20}
          strokeColor={colors.stroke}
          fillColor={colors.fill}
          strokeWidth={1.5}
        />
      </div>
      <div className="flex items-baseline gap-1">
        <span className={cn("text-2xl font-bold", colors.text)}>
          {typeof value === "number" ? value.toLocaleString(undefined, { maximumFractionDigits: 2 }) : value}
        </span>
        {unit && <span className="text-sm text-muted-foreground">{unit}</span>}
      </div>
    </div>
  );
}

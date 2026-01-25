"use client";

import { Badge } from "@/components/ui/badge";
import {
  AlertCircle,
  CheckCircle,
  Clock,
  GitBranch,
  Box,
  Cloud,
  FileText,
  Database,
  Settings,
} from "lucide-react";
import type { ArenaSource, ArenaSourceType } from "@/types/arena";

/**
 * Format a date string into a human-readable format.
 * @param dateString - ISO date string
 * @param includeYear - Whether to include the year in the output
 */
export function formatDate(dateString?: string, includeYear = false): string {
  if (!dateString) return "-";
  const date = new Date(dateString);
  const options: Intl.DateTimeFormatOptions = {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  };
  if (includeYear) {
    options.year = "numeric";
  }
  return date.toLocaleDateString("en-US", options);
}

/**
 * Format a Go duration string (e.g., "5m", "1h") to human-readable format.
 */
export function formatInterval(interval?: string): string {
  if (!interval) return "-";
  const match = RegExp(/^(\d+)([smhd])$/).exec(interval);
  if (!match) return interval;
  const [, value, unit] = match;
  const units: Record<string, string> = {
    s: "sec",
    m: "min",
    h: "hour",
    d: "day",
  };
  return `${value} ${units[unit]}${Number.parseInt(value) > 1 ? "s" : ""}`;
}

/**
 * Format bytes into a human-readable format.
 */
export function formatBytes(bytes?: number): string {
  if (bytes === undefined || bytes === null) return "-";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/**
 * Get the icon component for a source type.
 * @param type - The source type
 * @param size - Icon size: "sm" (4), "md" (5), or custom className
 */
export function getSourceTypeIcon(type?: ArenaSourceType, size: "sm" | "md" = "sm") {
  const sizeClass = size === "sm" ? "h-4 w-4" : "h-5 w-5";

  switch (type) {
    case "git":
      return <GitBranch className={sizeClass} />;
    case "oci":
      return <Box className={sizeClass} />;
    case "s3":
      return <Cloud className={sizeClass} />;
    case "configmap":
      return <FileText className={sizeClass} />;
    default:
      return size === "sm"
        ? <Database className={sizeClass} />
        : <Settings className={sizeClass} />;
  }
}

/**
 * Get a badge component showing the source type with icon.
 */
export function getSourceTypeBadge(type?: ArenaSourceType) {
  const colors: Record<ArenaSourceType, string> = {
    git: "border-green-500 text-green-600",
    oci: "border-blue-500 text-blue-600",
    s3: "border-orange-500 text-orange-600",
    configmap: "border-purple-500 text-purple-600",
  };

  return (
    <Badge variant="outline" className={type ? colors[type] : ""}>
      {getSourceTypeIcon(type, "sm")}
      <span className="ml-1 capitalize">{type || "Unknown"}</span>
    </Badge>
  );
}

/**
 * Get a badge component showing the status phase.
 */
export function getStatusBadge(phase?: string) {
  switch (phase) {
    case "Ready":
      return (
        <Badge variant="default" className="bg-green-500">
          <CheckCircle className="h-3 w-3 mr-1" /> Ready
        </Badge>
      );
    case "Failed":
      return (
        <Badge variant="destructive">
          <AlertCircle className="h-3 w-3 mr-1" /> Failed
        </Badge>
      );
    case "Pending":
      return (
        <Badge variant="outline">
          <Clock className="h-3 w-3 mr-1" /> Pending
        </Badge>
      );
    default:
      return <Badge variant="outline">{phase || "Unknown"}</Badge>;
  }
}

/**
 * Get the URL or reference string for a source.
 */
export function getSourceUrl(source: ArenaSource): string {
  const { spec } = source;
  if (spec.type === "git" && spec.git) {
    return spec.git.url;
  }
  if (spec.type === "oci" && spec.oci) {
    return spec.oci.url;
  }
  if (spec.type === "s3" && spec.s3) {
    const prefix = spec.s3.prefix ? "/" + spec.s3.prefix : "";
    return `s3://${spec.s3.bucket}${prefix}`;
  }
  if (spec.type === "configmap" && spec.configMap) {
    return spec.configMap.name;
  }
  return "-";
}

/**
 * Get an icon for a condition based on its status.
 */
export function getConditionIcon(status: string) {
  if (status === "True") {
    return <CheckCircle className="h-4 w-4 text-green-500" />;
  }
  if (status === "False") {
    return <AlertCircle className="h-4 w-4 text-red-500" />;
  }
  return <Clock className="h-4 w-4 text-yellow-500" />;
}

/**
 * CategoryBadge — colored badge for memory consent categories.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { Badge } from "@/components/ui/badge";

interface CategoryConfig {
  bg: string;
  text: string;
  label: string;
}

const CATEGORY_CONFIG: Record<string, CategoryConfig> = {
  "memory:identity": {
    bg: "bg-blue-100 dark:bg-blue-900/50",
    text: "text-blue-700 dark:text-blue-300",
    label: "Identity",
  },
  "memory:context": {
    bg: "bg-gray-100 dark:bg-gray-800",
    text: "text-gray-700 dark:text-gray-300",
    label: "Context",
  },
  "memory:health": {
    bg: "bg-red-100 dark:bg-red-900/50",
    text: "text-red-700 dark:text-red-300",
    label: "Health",
  },
  "memory:location": {
    bg: "bg-green-100 dark:bg-green-900/50",
    text: "text-green-700 dark:text-green-300",
    label: "Location",
  },
  "memory:preferences": {
    bg: "bg-purple-100 dark:bg-purple-900/50",
    text: "text-purple-700 dark:text-purple-300",
    label: "Preferences",
  },
  "memory:history": {
    bg: "bg-amber-100 dark:bg-amber-900/50",
    text: "text-amber-700 dark:text-amber-300",
    label: "History",
  },
};

const DEFAULT_CONFIG: CategoryConfig = {
  bg: "bg-gray-100 dark:bg-gray-800",
  text: "text-gray-700 dark:text-gray-300",
  label: "Unknown",
};

const HEX_COLORS: Record<string, string> = {
  "memory:identity": "#3b82f6",    // blue
  "memory:context": "#6b7280",     // gray
  "memory:health": "#ef4444",      // red
  "memory:location": "#22c55e",    // green
  "memory:preferences": "#a855f7", // purple
  "memory:history": "#f59e0b",     // amber
};

const FALLBACK_HEX = "#6b7280";

export function CategoryBadge({ category }: { category?: string }) {
  const config = (category && CATEGORY_CONFIG[category]) || DEFAULT_CONFIG;
  return (
    <Badge
      variant="outline"
      className={`${config.bg} ${config.text} border-0 text-xs`}
      data-testid="category-badge"
    >
      {config.label}
    </Badge>
  );
}

/** Return a hex color for @xyflow node backgrounds. */
export function getCategoryColor(category?: string): string {
  return (category && HEX_COLORS[category]) || FALLBACK_HEX;
}

/** Return a human-readable label for a consent category. */
export function getCategoryLabel(category?: string): string {
  return (category && CATEGORY_CONFIG[category]?.label) || "Context";
}

/**
 * CategoryBadge — colored badge for memory consent categories.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { Badge } from "@/components/ui/badge";
import {
  getCategoryClasses,
  getCategoryLabel,
  categoryColorVar,
  isKnownCategory,
} from "@/lib/colors/category";

export function CategoryBadge({ category }: { category?: string }) {
  const { bg, text } = getCategoryClasses(category);
  const label = isKnownCategory(category) ? getCategoryLabel(category) : "Unknown";
  return (
    <Badge
      variant="outline"
      className={`${bg} ${text} border-0 text-xs`}
      data-testid="category-badge"
    >
      {label}
    </Badge>
  );
}

/** CSS color for a category (themeable) — e.g. @xyflow node backgrounds. */
export function getCategoryColor(category?: string): string {
  return categoryColorVar(category);
}

export { getCategoryLabel };

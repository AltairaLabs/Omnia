/**
 * Categorical colors for memory consent categories (and other entity
 * categories). Single source of truth: a category maps to an index into the
 * SI-tunable --category-1..8 token palette. Derive the form you need:
 *   - categoryColorVar  -> "var(--category-N)"  (CSS / recharts / inline style)
 *   - getCategoryClasses -> token utility classes (HTML badges)
 *   - categoryColorHex  -> concrete hex matching the token default (canvas,
 *     which cannot resolve CSS variables)
 */

const NEUTRAL_INDEX = 8;

/** Memory category -> category token index. Hues roughly preserved. */
const MEMORY_CATEGORY_INDEX: Record<string, number> = {
  "memory:identity": 1,
  "memory:preferences": 2,
  "memory:history": 4,
  "memory:location": 5,
  "memory:health": 7,
  "memory:context": NEUTRAL_INDEX,
};

/** Light-mode defaults of --category-1..8 (kept in sync with globals.css). */
const CATEGORY_HEX: Record<number, string> = {
  1: "#3B82F6",
  2: "#8B5CF6",
  3: "#EC4899",
  4: "#F59E0B",
  5: "#10B981",
  6: "#06B6D4",
  7: "#EF4444",
  8: "#6B7280",
};

const CATEGORY_LABELS: Record<string, string> = {
  "memory:identity": "Identity",
  "memory:context": "Context",
  "memory:health": "Health",
  "memory:location": "Location",
  "memory:preferences": "Preferences",
  "memory:history": "History",
};

export function categoryIndex(category?: string): number {
  return (category && MEMORY_CATEGORY_INDEX[category]) || NEUTRAL_INDEX;
}

/** True if the category is a recognized memory category. */
export function isKnownCategory(category?: string): boolean {
  return category != null && category in MEMORY_CATEGORY_INDEX;
}

/** CSS variable for a category — themeable; use in CSS/recharts/inline style. */
export function categoryColorVar(category?: string): string {
  return `var(--category-${categoryIndex(category)})`;
}

/** Concrete hex for a category — for canvas, which can't resolve var(). */
export function categoryColorHex(category?: string): string {
  return CATEGORY_HEX[categoryIndex(category)];
}

/** Token utility classes for a category badge. */
export function getCategoryClasses(category?: string): { bg: string; text: string } {
  const n = categoryIndex(category);
  return { bg: `bg-category-${n}/15`, text: `text-category-${n}` };
}

/** Human-readable label for a consent category. */
export function getCategoryLabel(category?: string): string {
  return (category && CATEGORY_LABELS[category]) || "Context";
}

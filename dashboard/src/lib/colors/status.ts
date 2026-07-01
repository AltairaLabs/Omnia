/**
 * Semantic status colors.
 *
 * Central mapping from a domain-agnostic status kind to design tokens, so
 * status coloring (success / warning / info / error / neutral) is driven by
 * CSS variables and re-themes with a white-label brand — never by hardcoded
 * Tailwind palette classes.
 */

export type StatusKind = "success" | "warning" | "info" | "error" | "neutral";

const VAR_BY_KIND: Record<StatusKind, string> = {
  success: "var(--success)",
  warning: "var(--warning)",
  info: "var(--info)",
  error: "var(--destructive)",
  neutral: "var(--muted-foreground)",
};

const TOKEN_BY_KIND: Record<StatusKind, string> = {
  success: "success",
  warning: "warning",
  info: "info",
  error: "destructive",
  neutral: "muted-foreground",
};

/** Returns the CSS `var(--…)` reference for a status kind. */
export function getStatusColorVar(kind: StatusKind): string {
  return VAR_BY_KIND[kind];
}

/** Returns token utility classes (text/bg/border) for a status kind. */
export function getStatusClasses(kind: StatusKind): {
  text: string;
  bg: string;
  border: string;
} {
  const token = TOKEN_BY_KIND[kind];
  return {
    text: `text-${token}`,
    bg: `bg-${token}/15`,
    border: `border-${token}/30`,
  };
}

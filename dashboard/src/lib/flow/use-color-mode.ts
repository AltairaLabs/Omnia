"use client";

import { useTheme } from "next-themes";

/**
 * React Flow color mode derived from the active next-themes theme, so the graph
 * chrome (controls, minimap, background, default edges) follows light/dark.
 */
export function useFlowColorMode(): "dark" | "light" {
  const { resolvedTheme } = useTheme();
  return resolvedTheme === "dark" ? "dark" : "light";
}

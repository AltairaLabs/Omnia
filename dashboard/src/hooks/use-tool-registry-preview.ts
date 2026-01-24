"use client";

import { useMemo } from "react";
import { useToolRegistries } from "./use-tool-registries";
import type { ToolRegistry } from "@/types";
import {
  matchesLabelSelector,
  type LabelSelectorValue,
} from "@/components/ui/k8s-label-selector";

export interface ToolRegistryPreviewResult {
  /** All registries that match the selector */
  matchingRegistries: ToolRegistry[];
  /** Count of matching registries */
  matchCount: number;
  /** Total tools across matching registries */
  totalToolsCount: number;
  /** All available registries */
  allRegistries: ToolRegistry[];
  /** Total count of registries */
  totalCount: number;
  /** Whether data is loading */
  isLoading: boolean;
  /** Error if any */
  error: Error | null;
  /** Available labels from all registries (for autocomplete) */
  availableLabels: Record<string, string[]>;
}

/**
 * Hook to preview which tool registries match a given label selector.
 * Uses client-side filtering on the already-fetched registries list.
 *
 * @param selector - The label selector to match against registries
 * @returns Preview result with matching registries and metadata
 */
export function useToolRegistryPreview(
  selector: LabelSelectorValue | undefined
): ToolRegistryPreviewResult {
  const { data: registries, isLoading, error } = useToolRegistries();

  // Compute matching registries client-side
  const matchingRegistries = useMemo(() => {
    if (!registries || !selector) return [];

    // If selector is empty (no matchLabels and no matchExpressions), return all
    const hasSelector =
      (selector.matchLabels && Object.keys(selector.matchLabels).length > 0) ||
      (selector.matchExpressions && selector.matchExpressions.length > 0);

    if (!hasSelector) return registries;

    return registries.filter((registry) =>
      matchesLabelSelector(registry.metadata.labels, selector)
    );
  }, [registries, selector]);

  // Calculate total tools across matching registries
  const totalToolsCount = useMemo(() => {
    return matchingRegistries.reduce(
      (sum, registry) => sum + (registry.status?.discoveredToolsCount || 0),
      0
    );
  }, [matchingRegistries]);

  // Extract available labels from all registries (for autocomplete)
  const availableLabels = useMemo(() => {
    if (!registries) return {};

    const labelsMap: Record<string, Set<string>> = {};

    for (const registry of registries) {
      const labels = registry.metadata.labels || {};
      for (const [key, value] of Object.entries(labels)) {
        if (!labelsMap[key]) {
          labelsMap[key] = new Set();
        }
        labelsMap[key].add(value);
      }
    }

    // Convert Sets to sorted arrays
    const result: Record<string, string[]> = {};
    for (const [key, values] of Object.entries(labelsMap)) {
      result[key] = Array.from(values).sort();
    }

    return result;
  }, [registries]);

  return {
    matchingRegistries,
    matchCount: matchingRegistries.length,
    totalToolsCount,
    allRegistries: registries || [],
    totalCount: registries?.length || 0,
    isLoading,
    error: error as Error | null,
    availableLabels,
  };
}

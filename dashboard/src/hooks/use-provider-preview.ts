"use client";

import { useMemo } from "react";
import { useProviders } from "./use-providers";
import type { Provider } from "@/types";
import {
  matchesLabelSelector,
  type LabelSelectorValue,
} from "@/components/ui/k8s-label-selector";

export interface ProviderPreviewResult {
  /** All providers that match the selector */
  matchingProviders: Provider[];
  /** Count of matching providers */
  matchCount: number;
  /** All available providers */
  allProviders: Provider[];
  /** Total count of providers */
  totalCount: number;
  /** Whether data is loading */
  isLoading: boolean;
  /** Error if any */
  error: Error | null;
  /** Available labels from all providers (for autocomplete) */
  availableLabels: Record<string, string[]>;
}

/**
 * Hook to preview which providers match a given label selector.
 * Uses client-side filtering on the already-fetched providers list.
 *
 * @param selector - The label selector to match against providers
 * @returns Preview result with matching providers and metadata
 */
export function useProviderPreview(
  selector: LabelSelectorValue | undefined
): ProviderPreviewResult {
  const { data: providers, isLoading, error } = useProviders();

  // Compute matching providers client-side
  const matchingProviders = useMemo(() => {
    if (!providers || !selector) return [];

    // If selector is empty (no matchLabels and no matchExpressions), return all
    const hasSelector =
      (selector.matchLabels && Object.keys(selector.matchLabels).length > 0) ||
      (selector.matchExpressions && selector.matchExpressions.length > 0);

    if (!hasSelector) return providers;

    return providers.filter((provider) =>
      matchesLabelSelector(provider.metadata.labels, selector)
    );
  }, [providers, selector]);

  // Extract available labels from all providers (for autocomplete)
  const availableLabels = useMemo(() => {
    if (!providers) return {};

    const labelsMap: Record<string, Set<string>> = {};

    for (const provider of providers) {
      const labels = provider.metadata.labels || {};
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
      result[key] = Array.from(values).sort((a, b) => a.localeCompare(b));
    }

    return result;
  }, [providers]);

  return {
    matchingProviders,
    matchCount: matchingProviders.length,
    allProviders: providers || [],
    totalCount: providers?.length || 0,
    isLoading,
    error: error as Error | null,
    availableLabels,
  };
}

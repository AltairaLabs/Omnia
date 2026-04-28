"use client";

/**
 * Hook for discovering eval groups available to an agent.
 *
 * Returns the union of:
 *   - Built-in groups: `default`, `fast-running`, `long-running`, `external`.
 *     PromptKit auto-classifies handlers into the latter three; every eval
 *     also carries `default`. These are always offered as options even when
 *     the pack declares no custom groups.
 *   - Custom groups declared on any eval in the pack — pack-level
 *     `evals[].groups` plus prompt-level `prompts[*].evals[].groups`.
 *
 * Supports the dashboard's eval routing UI (#988) without requiring an
 * extra dashboard endpoint — the PromptPack content is already
 * fetchable via the existing data-service path.
 */

import { useMemo } from "react";
import { usePromptPackContent } from "./use-promptpack-content";
import type { PromptPackContent } from "@/lib/data/types";

export const BUILTIN_EVAL_GROUPS = [
  "default",
  "fast-running",
  "long-running",
  "external",
] as const;

export interface UseEvalGroupsResult {
  /** Sorted, deduped list of group names available for selection.
   *  Always includes the four built-in groups, plus pack-declared
   *  custom groups when a packName is supplied. */
  groups: string[];
  /** True while the pack content is loading; the caller can render a
   *  skeleton without flickering between built-in-only and full lists. */
  isLoading: boolean;
}

export function useEvalGroups(
  workspace: string,
  packName: string | undefined,
): UseEvalGroupsResult {
  const { data: content, isLoading } = usePromptPackContent(packName ?? "", workspace);

  const groups = useMemo(() => {
    const set = new Set<string>(BUILTIN_EVAL_GROUPS);
    if (content) {
      for (const g of collectPackGroups(content)) {
        set.add(g);
      }
    }
    return Array.from(set).sort();
  }, [content]);

  return { groups, isLoading: !!packName && isLoading };
}

/**
 * collectPackGroups walks every eval definition in the pack — both
 * pack-level and prompt-level — and yields the group names declared on
 * each. Exported separately so tests can pin the collection rule
 * without going through React Query.
 */
export function collectPackGroups(content: PromptPackContent | null | undefined): string[] {
  if (!content) return [];
  const out: string[] = [];
  for (const e of content.evals ?? []) {
    if (e.groups) out.push(...e.groups);
  }
  for (const promptId of Object.keys(content.prompts ?? {})) {
    const prompt = content.prompts?.[promptId];
    for (const e of prompt?.evals ?? []) {
      if (e.groups) out.push(...e.groups);
    }
  }
  return out;
}

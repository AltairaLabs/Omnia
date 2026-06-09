"use client";

/**
 * Cost data for the current workspace.
 *
 * Reads exact aggregates from session-api via the workspace costs route
 * (product data — see CLAUDE.md → Observability Boundaries). Returns mock
 * data in demo mode.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { useWorkspace } from "@/contexts/workspace-context";
import { useRuntimeConfig } from "@/hooks/core";
import { getMockCostData } from "@/lib/data/mock-service";
import { useWorkspaceCosts, type WorkspaceCostData } from "./use-workspace-costs";

export { useWorkspaceCosts } from "./use-workspace-costs";
export type { WorkspaceCostData } from "./use-workspace-costs";

/** Result shape consumed by the Home + Costs pages. */
interface UseCostsResult {
  data: WorkspaceCostData | undefined;
  isLoading: boolean;
  error: unknown;
}

/**
 * Cost data for the current workspace. Reads exact aggregates from session-api
 * via the workspace costs route; returns mock data in demo mode.
 */
export function useCosts(): UseCostsResult {
  const { config } = useRuntimeConfig();
  const { currentWorkspace } = useWorkspace();
  const query = useWorkspaceCosts(currentWorkspace?.name, { enabled: !config.demoMode });

  if (config.demoMode) {
    return { data: getMockCostData() as WorkspaceCostData, isLoading: false, error: null };
  }
  return query;
}

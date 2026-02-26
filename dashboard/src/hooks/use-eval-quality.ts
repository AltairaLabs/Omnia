/**
 * Hooks for agent quality dashboard with eval pass rates.
 *
 * Uses SessionApiService directly since eval quality data is not part of
 * the DataService interface (session-api specific).
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import { useQuery } from "@tanstack/react-query";
import { SessionApiService } from "@/lib/data/session-api-service";
import { useWorkspace } from "@/contexts/workspace-context";

export interface EvalSummaryParams {
  agentName?: string;
  evalType?: string;
  createdAfter?: string;
  createdBefore?: string;
}

export interface EvalListParams {
  agentName?: string;
  evalId?: string;
  evalType?: string;
  passed?: boolean;
  limit?: number;
  offset?: number;
}

/**
 * Fetch eval summary (pass rates per eval_id) for the current workspace.
 */
export function useEvalSummary(params?: EvalSummaryParams) {
  const { currentWorkspace } = useWorkspace();

  return useQuery({
    queryKey: [
      "eval-summary",
      currentWorkspace?.name,
      params?.agentName,
      params?.evalType,
      params?.createdAfter,
      params?.createdBefore,
    ],
    queryFn: async () => {
      if (!currentWorkspace) {
        return [];
      }
      const service = new SessionApiService();
      return service.getEvalResultsSummary(currentWorkspace.name, params);
    },
    enabled: !!currentWorkspace,
    staleTime: 30000,
  });
}

/**
 * Fetch recent eval failures for the current workspace.
 */
export function useRecentEvalFailures(params?: EvalListParams) {
  const { currentWorkspace } = useWorkspace();

  const mergedParams = { passed: false, limit: 10, ...params };

  return useQuery({
    queryKey: [
      "eval-failures",
      currentWorkspace?.name,
      mergedParams.agentName,
      mergedParams.evalId,
      mergedParams.evalType,
      mergedParams.limit,
      mergedParams.offset,
    ],
    queryFn: async () => {
      if (!currentWorkspace) {
        return { evalResults: [], total: 0 };
      }
      const service = new SessionApiService();
      return service.getEvalResults(currentWorkspace.name, mergedParams);
    },
    enabled: !!currentWorkspace,
    staleTime: 30000,
  });
}

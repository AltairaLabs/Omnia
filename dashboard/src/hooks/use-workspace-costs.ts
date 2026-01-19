"use client";

/**
 * Hook for fetching workspace cost data.
 * Queries the workspace-scoped cost API endpoint.
 */

import { useQuery } from "@tanstack/react-query";
import type { CostData } from "@/lib/data/types";

/**
 * Workspace cost data with budget information.
 */
export interface WorkspaceCostData extends CostData {
  /** Budget configuration from workspace (if set) */
  budget?: {
    dailyBudget?: string;
    monthlyBudget?: string;
    dailyUsedPercent?: number;
    monthlyUsedPercent?: number;
  };
}

/**
 * Options for fetching workspace costs.
 */
interface UseWorkspaceCostsOptions {
  /** Whether to enable the query */
  enabled?: boolean;
  /** Refetch interval in milliseconds (default: 5 minutes) */
  refetchInterval?: number;
}

/**
 * Fetch costs from the workspace API.
 */
async function fetchWorkspaceCosts(workspaceName: string): Promise<WorkspaceCostData> {
  const response = await fetch(`/api/workspaces/${encodeURIComponent(workspaceName)}/costs`);

  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: "Failed to fetch workspace costs" }));
    throw new Error(error.message || "Failed to fetch workspace costs");
  }

  return response.json();
}

/**
 * Hook to fetch cost data for a specific workspace.
 *
 * @param workspaceName - The name of the workspace to fetch costs for
 * @param options - Query options
 *
 * @example
 * ```tsx
 * function WorkspaceCosts({ workspace }: { workspace: string }) {
 *   const { data, isLoading, error } = useWorkspaceCosts(workspace);
 *
 *   if (isLoading) return <Spinner />;
 *   if (error) return <ErrorMessage error={error} />;
 *   if (!data?.available) return <CostUnavailable reason={data?.reason} />;
 *
 *   return (
 *     <div>
 *       <p>Total Cost: ${data.summary.totalCost.toFixed(2)}</p>
 *       {data.budget && (
 *         <Progress value={data.budget.dailyUsedPercent} />
 *       )}
 *     </div>
 *   );
 * }
 * ```
 */
export function useWorkspaceCosts(
  workspaceName: string | null | undefined,
  options: UseWorkspaceCostsOptions = {}
) {
  const { enabled = true, refetchInterval = 5 * 60 * 1000 } = options;

  return useQuery({
    queryKey: ["workspace-costs", workspaceName],
    queryFn: () => fetchWorkspaceCosts(workspaceName!),
    enabled: enabled && !!workspaceName,
    refetchInterval,
    staleTime: 60000, // Cache for 1 minute
  });
}

/**
 * Hook for fetching Kubernetes events for an agent.
 *
 * Uses the DataService abstraction which automatically switches
 * between mock data (demo mode) and real K8s data (live mode).
 */

import { useQuery } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";

export type { K8sEvent } from "@/lib/data";

/**
 * Hook to fetch K8s events for a specific agent.
 *
 * In demo mode, returns realistic mock events.
 * In live mode, queries the workspace API for real K8s events.
 *
 * @param agentName - Name of the agent
 * @param workspace - Workspace name (not K8s namespace)
 */
export function useAgentEvents(agentName: string, workspace: string) {
  const dataService = useDataService();

  return useQuery({
    queryKey: ["agent-events", workspace, agentName],
    queryFn: () => dataService.getAgentEvents(workspace, agentName),
    enabled: !!agentName && !!workspace,
    refetchInterval: 30000, // Refresh every 30 seconds
    staleTime: 15000,
  });
}

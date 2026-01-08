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
 * In live mode, queries the operator API for real K8s events.
 *
 * @param agentName - Name of the agent
 * @param namespace - Namespace of the agent
 */
export function useAgentEvents(agentName: string, namespace: string) {
  const dataService = useDataService();

  return useQuery({
    queryKey: ["agent-events", namespace, agentName],
    queryFn: () => dataService.getAgentEvents(namespace, agentName),
    enabled: !!agentName && !!namespace,
    refetchInterval: 30000, // Refresh every 30 seconds
    staleTime: 15000,
  });
}

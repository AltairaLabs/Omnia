"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService, type AgentRuntime as ServiceAgentRuntime } from "@/lib/data";
import { useWorkspace } from "@/contexts/workspace-context";
import type { AgentRuntime, AgentRuntimePhase } from "@/types";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";

interface UseAgentsOptions {
  phase?: AgentRuntimePhase;
  /** Override workspace name (defaults to current workspace). */
  workspaceName?: string;
}

/**
 * Fetch agents for a workspace.
 * Defaults to the current workspace; pass `workspaceName` to fetch from a different one.
 * The DataService handles whether to use mock data (demo mode) or real API (live mode).
 */
export function useAgents(options: UseAgentsOptions = {}) {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();

  // Extract for stable query key (avoid object comparison issues)
  const { phase, workspaceName } = options;
  const effectiveWorkspace = workspaceName || currentWorkspace?.name;

  return useQuery({
    queryKey: ["agents", effectiveWorkspace, phase, service.name],
    queryFn: async (): Promise<AgentRuntime[]> => {
      if (!effectiveWorkspace) {
        return [];
      }

      // DataService handles demo vs live mode internally
      let agents = await service.getAgents(effectiveWorkspace) as unknown as AgentRuntime[];

      // Client-side filtering for phase
      if (phase) {
        agents = agents.filter((a) => a.status?.phase === phase);
      }

      return agents;
    },
    enabled: !!effectiveWorkspace,
    // Ensure fresh data on workspace change
    staleTime: DEFAULT_STALE_TIME,
    refetchOnMount: true, // Only refetch if stale
  });
}

/**
 * Fetch a single agent by name.
 * Uses current workspace context.
 *
 * @param name - Agent name
 * @param _namespace - Deprecated parameter, kept for backwards compatibility. Use workspace context instead.
 */
export function useAgent(name: string, _namespace?: string) {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();

  return useQuery({
    queryKey: ["agent", currentWorkspace?.name, name, service.name],
    queryFn: async (): Promise<AgentRuntime | null> => {
      if (!currentWorkspace) {
        return null;
      }

      // DataService handles demo vs live mode internally
      const agent = await service.getAgent(currentWorkspace.name, name) as ServiceAgentRuntime | undefined;
      return (agent as unknown as AgentRuntime) || null;
    },
    enabled: !!name && !!currentWorkspace,
    staleTime: DEFAULT_STALE_TIME,
    refetchOnMount: true, // Only refetch if stale
  });
}

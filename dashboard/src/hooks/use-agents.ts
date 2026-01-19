"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService, type AgentRuntime as ServiceAgentRuntime } from "@/lib/data";
import { useWorkspace } from "@/contexts/workspace-context";
import type { AgentRuntime, AgentRuntimePhase } from "@/types";

interface UseAgentsOptions {
  phase?: AgentRuntimePhase;
}

/**
 * Fetch agents for the current workspace.
 * The DataService handles whether to use mock data (demo mode) or real API (live mode).
 */
export function useAgents(options: UseAgentsOptions = {}) {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();

  // Extract phase for stable query key (avoid object comparison issues)
  const { phase } = options;

  return useQuery({
    queryKey: ["agents", currentWorkspace?.name, phase, service.name],
    queryFn: async (): Promise<AgentRuntime[]> => {
      if (!currentWorkspace) {
        return [];
      }

      // DataService handles demo vs live mode internally
      let agents = await service.getAgents(currentWorkspace.name) as unknown as AgentRuntime[];

      // Client-side filtering for phase
      if (phase) {
        agents = agents.filter((a) => a.status?.phase === phase);
      }

      return agents;
    },
    enabled: !!currentWorkspace,
    // Ensure fresh data on workspace change
    staleTime: 0,
    refetchOnMount: "always",
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
    staleTime: 0,
    refetchOnMount: "always",
  });
}

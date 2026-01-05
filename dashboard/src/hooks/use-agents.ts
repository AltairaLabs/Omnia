"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchAgents, fetchAgent } from "@/lib/api-client";
import type { AgentRuntime, AgentRuntimePhase } from "@/types";

interface UseAgentsOptions {
  namespace?: string;
  phase?: AgentRuntimePhase;
}

export function useAgents(options: UseAgentsOptions = {}) {
  return useQuery({
    queryKey: ["agents", options],
    queryFn: async (): Promise<AgentRuntime[]> => {
      let agents = await fetchAgents(options.namespace);

      // Client-side filtering for phase (could also be done server-side)
      if (options.phase) {
        agents = agents.filter((a) => a.status?.phase === options.phase);
      }

      return agents;
    },
  });
}

export function useAgent(name: string, namespace: string = "production") {
  return useQuery({
    queryKey: ["agent", namespace, name],
    queryFn: async (): Promise<AgentRuntime | null> => {
      const agent = await fetchAgent(namespace, name);
      return agent || null;
    },
    enabled: !!name,
  });
}

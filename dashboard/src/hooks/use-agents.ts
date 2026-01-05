"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchAgents, fetchAgent } from "@/lib/api/client";
import type { AgentRuntime, AgentRuntimePhase } from "@/types";

interface UseAgentsOptions {
  namespace?: string;
  phase?: AgentRuntimePhase;
}

export function useAgents(options: UseAgentsOptions = {}) {
  return useQuery({
    queryKey: ["agents", options],
    queryFn: async (): Promise<AgentRuntime[]> => {
      const response = await fetchAgents(options.namespace);
      // Cast to local types (API response is compatible but types are generated separately)
      let agents = response as unknown as AgentRuntime[];

      // Client-side filtering for phase
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
      const response = await fetchAgent(namespace, name);
      // Cast to local type
      return (response as unknown as AgentRuntime) || null;
    },
    enabled: !!name,
  });
}

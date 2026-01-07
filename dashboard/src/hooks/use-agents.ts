"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";
import type { AgentRuntime, AgentRuntimePhase } from "@/types";

interface UseAgentsOptions {
  namespace?: string;
  phase?: AgentRuntimePhase;
}

export function useAgents(options: UseAgentsOptions = {}) {
  const service = useDataService();

  return useQuery({
    queryKey: ["agents", options, service.name],
    queryFn: async (): Promise<AgentRuntime[]> => {
      const response = await service.getAgents(options.namespace);
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
  const service = useDataService();

  return useQuery({
    queryKey: ["agent", namespace, name, service.name],
    queryFn: async (): Promise<AgentRuntime | null> => {
      const response = await service.getAgent(namespace, name);
      return (response as unknown as AgentRuntime) || null;
    },
    enabled: !!name,
  });
}

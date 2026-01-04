"use client";

import { useQuery } from "@tanstack/react-query";
import { mockAgentRuntimes } from "@/lib/mock-data";
import type { AgentRuntime, AgentRuntimePhase } from "@/types";

interface UseAgentsOptions {
  namespace?: string;
  phase?: AgentRuntimePhase;
}

export function useAgents(options: UseAgentsOptions = {}) {
  return useQuery({
    queryKey: ["agents", options],
    queryFn: async (): Promise<AgentRuntime[]> => {
      // Simulate network delay
      await new Promise((resolve) => setTimeout(resolve, 300));

      let agents = [...mockAgentRuntimes];

      if (options.namespace) {
        agents = agents.filter(
          (a) => a.metadata.namespace === options.namespace
        );
      }

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
      // Simulate network delay
      await new Promise((resolve) => setTimeout(resolve, 200));

      const agent = mockAgentRuntimes.find(
        (a) => a.metadata.name === name && a.metadata.namespace === namespace
      );

      return agent || null;
    },
    enabled: !!name,
  });
}

import { useMemo } from "react";
import { usePromptPackContent } from "@/hooks/resources";
import { deriveWorkloadTier, type WorkloadTier } from "@/components/workload-graph";

export interface WorkloadTierSummary {
  tier?: WorkloadTier;
  agents: number;
  tools: number;
  states: number;
  isLoading: boolean;
}

export function useWorkloadTier(name: string, namespace?: string): WorkloadTierSummary {
  const { data: content, isLoading } = usePromptPackContent(name, namespace);
  return useMemo(() => {
    if (!content) return { isLoading, agents: 0, tools: 0, states: 0 };
    const model = deriveWorkloadTier(content);
    return {
      tier: model.tier,
      agents: model.meta.counts.agents,
      tools: model.meta.counts.tools,
      states: model.meta.counts.states,
      isLoading,
    };
  }, [content, isLoading]);
}

import type { WorkloadTier } from "./types";

const TIER_LABELS: Record<WorkloadTier, string> = {
  single: "Single",
  workflow: "Workflow",
  multiagent: "Multi-agent",
};

// Human-facing label for a workload tier. Keep display strings here so every
// surface (graph, card, topology) stays consistent.
export function workloadTierLabel(tier: WorkloadTier): string {
  return TIER_LABELS[tier];
}

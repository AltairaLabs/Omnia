import type { SkillRef } from "@/types/prompt-pack";
import type { SkillSource } from "@/types/skill-source";
import type {
  WorkloadModel,
  WorkloadNode,
  WorkloadEdge,
  ResolutionStatus,
} from "../types";

// SkillSource refs are pack-level bindings the PromptPack controller resolves
// against cluster SkillSources. We render each as a first-class `skill` node
// attached to the workload's entry, resolved against the SkillSource status.

function phaseToResolution(phase: string | undefined, found: boolean): ResolutionStatus {
  if (!found) return "unavailable"; // referenced SkillSource doesn't exist
  if (phase === "Ready") return "resolved";
  if (phase === "Error") return "unavailable";
  return "unresolved"; // Pending | Initializing | Fetching
}

function skillBadgeLabel(phase: string, count?: number): string {
  if (phase === "Ready" && typeof count === "number") return `${phase} · ${count}`;
  return phase;
}

export function skillNodesAndEdges(
  skillRefs: SkillRef[] | undefined,
  sourcesByName: Map<string, SkillSource>,
  entryId: string | undefined,
): { nodes: WorkloadNode[]; edges: WorkloadEdge[] } {
  const nodes: WorkloadNode[] = [];
  const edges: WorkloadEdge[] = [];

  for (const ref of skillRefs ?? []) {
    const src = sourcesByName.get(ref.source);
    const phase = src?.status?.phase ?? "missing";
    const count = src?.status?.skillCount;
    const id = `skill:${ref.source}`;
    nodes.push({
      id,
      kind: "skill",
      label: ref.source,
      resolution: phaseToResolution(src?.status?.phase, !!src),
      badges: [{ icon: "skill", label: skillBadgeLabel(phase, count) }],
      detail: {
        skillSource: ref.source,
        include: ref.include,
        mountAs: ref.mountAs,
        skillCount: count,
        skillPhase: phase,
      },
    });
    if (entryId) {
      edges.push({
        id: `${entryId}--provides-->${id}`,
        source: entryId,
        target: id,
        style: "provides",
      });
    }
  }

  return { nodes, edges };
}

// attachSkills appends SkillSource skill nodes/edges to an already-derived
// model and updates the skill count. No-op when the pack declares no SkillRefs.
export function attachSkills(
  model: WorkloadModel,
  skillRefs: SkillRef[] | undefined,
  skillSources: SkillSource[] | undefined,
): WorkloadModel {
  if (!skillRefs?.length) return model;
  const entryId = model.nodes.find((n) => n.isEntry)?.id ?? model.nodes[0]?.id;
  const byName = new Map((skillSources ?? []).map((s) => [s.metadata.name, s]));
  const { nodes, edges } = skillNodesAndEdges(skillRefs, byName, entryId);
  return {
    ...model,
    nodes: [...model.nodes, ...nodes],
    edges: [...model.edges, ...edges],
    meta: {
      ...model.meta,
      counts: { ...model.meta.counts, skills: model.meta.counts.skills + nodes.length },
    },
  };
}

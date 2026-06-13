import type { SkillRef } from "@/types/prompt-pack";
import type { SkillSource } from "@/types/skill-source";
import type { WorkloadModel, WorkloadNode } from "../types";

// SkillSource refs are pack-level: the resolved skills are available to every
// agent/state in the workload (a per-state `skills:` filter narrows them, when
// present). Rather than a floating "skill source" node, we decorate each
// agent/state node with the source so its drawer can list the actual skills.

function skillBadgeLabel(phase: string, count: number | undefined): string {
  return count === undefined ? phase : `${count}`;
}

export function attachSkills(
  model: WorkloadModel,
  skillRefs: SkillRef[] | undefined,
  skillSources: SkillSource[] | undefined,
): WorkloadModel {
  if (!skillRefs?.length) return model;
  // Primary source for the badge/detail; multi-source packs surface the first.
  const ref = skillRefs[0];
  const src = (skillSources ?? []).find((s) => s.metadata.name === ref.source);
  const phase = src?.status?.phase ?? "missing";
  const count = src?.status?.skillCount;
  const label = skillBadgeLabel(phase, count);

  const decorate = (n: WorkloadNode): WorkloadNode => {
    if (n.kind !== "agent" && n.kind !== "state") return n;
    return {
      ...n,
      badges: n.badges.map((b) => (b.icon === "skill" ? { ...b, label } : b)),
      detail: {
        ...n.detail,
        skillSource: ref.source,
        mountAs: ref.mountAs,
        include: ref.include,
        skillCount: count,
        skillPhase: phase,
      },
    };
  };

  return {
    ...model,
    nodes: model.nodes.map(decorate),
    meta: {
      ...model.meta,
      counts: { ...model.meta.counts, skills: count ?? model.meta.counts.skills },
    },
  };
}

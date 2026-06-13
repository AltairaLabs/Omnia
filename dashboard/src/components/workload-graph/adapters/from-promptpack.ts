import type { PromptPackContent } from "@/lib/data/types";
import type { SkillRef } from "@/types/prompt-pack";
import type { SkillSource } from "@/types/skill-source";
import type { WorkloadModel } from "../types";
import { deriveWorkloadTier } from "../derive-tier";
import { attachSkills } from "./skills";

export interface PackWorkloadOptions {
  skillRefs?: SkillRef[];
  skillSources?: SkillSource[];
}

export function promptPackToWorkload(
  content: PromptPackContent | undefined,
  opts?: PackWorkloadOptions,
): WorkloadModel {
  const base = deriveWorkloadTier(content ?? {});
  return attachSkills(base, opts?.skillRefs, opts?.skillSources);
}

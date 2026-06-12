import type { PromptPackContent } from "@/lib/data/types";
import type { WorkloadModel } from "../types";
import { deriveWorkloadTier } from "../derive-tier";

export function promptPackToWorkload(
  content: PromptPackContent | undefined,
): WorkloadModel {
  return deriveWorkloadTier(content ?? {});
}

import { Badge } from "@/components/ui/badge";

const PACK_CLASS_LABEL = "omnia.altairalabs.ai/pack-class";
const ROLE_ANNOTATION = "omnia.altairalabs.ai/pack-role";

interface SystemPackBadgeProps {
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
}

/**
 * Renders a "System" badge when the AgentRuntime / PromptPack carries
 * the pack-class=system label (set by the consolidation reference
 * packs). Tooltip surfaces the pack-role annotation.
 *
 * See docs/local-backlog/2026-05-22-memory-consolidation-design.md
 * for the labelling convention.
 */
export function SystemPackBadge({
  labels,
  annotations,
}: Readonly<SystemPackBadgeProps>) {
  if (labels?.[PACK_CLASS_LABEL] !== "system") return null;
  const role = annotations?.[ROLE_ANNOTATION];
  return (
    <Badge variant="secondary" title={role}>
      System
    </Badge>
  );
}

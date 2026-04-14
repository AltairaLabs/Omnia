/**
 * API route for a specific workspace-scoped SkillSource. Issue #829.
 *
 * GET    /api/workspaces/:name/skills/:sourceName - Get skill source details
 * PUT    /api/workspaces/:name/skills/:sourceName - Update skill source
 * DELETE /api/workspaces/:name/skills/:sourceName - Delete skill source
 */

import { createItemRoutes } from "@/lib/api/crd-route-factory";
import { CRD_SKILL_SOURCES } from "@/lib/k8s/workspace-route-helpers";
import type { SkillSource } from "@/types/skill-source";

export const { GET, PUT, DELETE } = createItemRoutes<SkillSource>({
  kind: "SkillSource",
  plural: CRD_SKILL_SOURCES,
  resourceLabel: "Skill source",
  paramKey: "sourceName",
  errorLabel: "skill source",
});

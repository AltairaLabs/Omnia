/**
 * API route for workspace-scoped SkillSource CRDs. Issue #829.
 *
 * GET  /api/workspaces/:name/skills - List skill sources in workspace
 * POST /api/workspaces/:name/skills - Create a skill source
 */

import { createCollectionRoutes } from "@/lib/api/crd-route-factory";
import { CRD_SKILL_SOURCES } from "@/lib/k8s/workspace-route-helpers";
import type { SkillSource } from "@/types/skill-source";

export const { GET, POST } = createCollectionRoutes<SkillSource>({
  kind: "SkillSource",
  plural: CRD_SKILL_SOURCES,
  errorLabel: "skill sources",
});

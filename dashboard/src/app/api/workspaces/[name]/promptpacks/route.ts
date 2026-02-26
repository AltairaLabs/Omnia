/**
 * API routes for workspace prompt packs (PromptPack CRDs).
 *
 * GET /api/workspaces/:name/promptpacks - List prompt packs in workspace
 * POST /api/workspaces/:name/promptpacks - Create a new prompt pack
 *
 * Protected by workspace access checks.
 */

import { createCollectionRoutes } from "@/lib/api/crd-route-factory";
import { CRD_PROMPTPACKS } from "@/lib/k8s/workspace-route-helpers";
import type { PromptPack } from "@/lib/data/types";

export const { GET, POST } = createCollectionRoutes<PromptPack>({
  kind: "PromptPack",
  plural: CRD_PROMPTPACKS,
  errorLabel: "prompt packs",
});

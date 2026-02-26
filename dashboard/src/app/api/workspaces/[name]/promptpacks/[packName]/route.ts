/**
 * API routes for individual workspace prompt pack operations.
 *
 * GET /api/workspaces/:name/promptpacks/:packName - Get prompt pack details
 * PUT /api/workspaces/:name/promptpacks/:packName - Update prompt pack
 * DELETE /api/workspaces/:name/promptpacks/:packName - Delete prompt pack
 *
 * Protected by workspace access checks.
 */

import { createItemRoutes } from "@/lib/api/crd-route-factory";
import { CRD_PROMPTPACKS } from "@/lib/k8s/workspace-route-helpers";
import type { PromptPack } from "@/lib/data/types";

export const { GET, PUT, DELETE } = createItemRoutes<PromptPack>({
  kind: "PromptPack",
  plural: CRD_PROMPTPACKS,
  resourceLabel: "Prompt pack",
  paramKey: "packName",
  errorLabel: "this prompt pack",
});

/**
 * API route for workspace-scoped tool registries.
 *
 * GET /api/workspaces/:name/toolregistries - List tool registries in workspace
 * POST /api/workspaces/:name/toolregistries - Create a tool registry in workspace
 *
 * Tool registries can be workspace-scoped (in workspace namespace) or
 * shared (in omnia-system namespace). This endpoint manages workspace-scoped ones.
 */

import { createCollectionRoutes } from "@/lib/api/crd-route-factory";
import { CRD_TOOL_REGISTRIES } from "@/lib/k8s/workspace-route-helpers";
import type { ToolRegistry } from "@/lib/data/types";

export const { GET, POST } = createCollectionRoutes<ToolRegistry>({
  kind: "ToolRegistry",
  plural: CRD_TOOL_REGISTRIES,
  errorLabel: "tool registries",
});

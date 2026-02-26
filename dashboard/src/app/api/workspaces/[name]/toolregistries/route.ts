/**
 * API route for workspace-scoped tool registries.
 *
 * GET /api/workspaces/:name/toolregistries - List tool registries in workspace
 *
 * Tool registries can be workspace-scoped (in workspace namespace) or
 * shared (in omnia-system namespace). This endpoint returns workspace-scoped ones.
 */

import { createCollectionRoutes } from "@/lib/api/crd-route-factory";
import type { ToolRegistry } from "@/lib/data/types";

const routes = createCollectionRoutes<ToolRegistry>({
  kind: "ToolRegistry",
  plural: "toolregistries",
  errorLabel: "tool registries",
});

export const GET = routes.GET;

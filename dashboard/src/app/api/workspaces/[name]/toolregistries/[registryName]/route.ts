/**
 * API route for a specific workspace-scoped tool registry.
 *
 * GET /api/workspaces/:name/toolregistries/:registryName - Get tool registry details
 *
 * Protected by workspace access checks.
 */

import { createItemRoutes } from "@/lib/api/crd-route-factory";
import type { ToolRegistry } from "@/lib/data/types";

const routes = createItemRoutes<ToolRegistry>({
  kind: "ToolRegistry",
  plural: "toolregistries",
  resourceLabel: "Tool registry",
  paramKey: "registryName",
  errorLabel: "tool registry",
});

export const GET = routes.GET;

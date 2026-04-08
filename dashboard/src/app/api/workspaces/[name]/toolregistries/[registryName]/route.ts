/**
 * API route for a specific workspace-scoped tool registry.
 *
 * GET    /api/workspaces/:name/toolregistries/:registryName - Get tool registry details
 * PUT    /api/workspaces/:name/toolregistries/:registryName - Update tool registry
 * DELETE /api/workspaces/:name/toolregistries/:registryName - Delete tool registry
 *
 * Protected by workspace access checks.
 */

import { createItemRoutes } from "@/lib/api/crd-route-factory";
import { CRD_TOOL_REGISTRIES } from "@/lib/k8s/workspace-route-helpers";
import type { ToolRegistry } from "@/lib/data/types";

export const { GET, PUT, DELETE } = createItemRoutes<ToolRegistry>({
  kind: "ToolRegistry",
  plural: CRD_TOOL_REGISTRIES,
  resourceLabel: "Tool registry",
  paramKey: "registryName",
  errorLabel: "tool registry",
});

/**
 * API route for getting a specific shared tool registry.
 *
 * GET /api/shared/toolregistries/:registryName - Get tool registry details
 *
 * Tool registries are cluster-wide resources that define available tools.
 * Accessible to all users (including anonymous) - these are read-only
 * configuration resources.
 */

import { createSharedItemRoutes } from "@/lib/api/crd-route-factory";
import type { ToolRegistry } from "@/lib/data/types";

export const { GET } = createSharedItemRoutes<ToolRegistry>({
  plural: "toolregistries",
  paramKey: "registryName",
  resourceLabel: "Tool registry",
  errorLabel: "tool registry",
});

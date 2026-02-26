/**
 * API route for listing shared tool registries.
 *
 * GET /api/shared/toolregistries - List all shared tool registries
 *
 * Tool registries are cluster-wide resources that define available tools.
 * Accessible to all users (including anonymous) - these are read-only
 * configuration resources.
 */

import { createSharedCollectionRoutes } from "@/lib/api/crd-route-factory";
import type { ToolRegistry } from "@/lib/data/types";

export const { GET } = createSharedCollectionRoutes<ToolRegistry>({
  plural: "toolregistries",
  errorLabel: "tool registries",
});

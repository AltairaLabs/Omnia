/**
 * API route for workspace-scoped tool policies.
 *
 * GET  /api/workspaces/:name/toolpolicies - List tool policies in workspace
 * POST /api/workspaces/:name/toolpolicies - Create a tool policy in workspace
 */

import { createCollectionRoutes } from "@/lib/api/crd-route-factory";
import { CRD_TOOL_POLICIES } from "@/lib/k8s/workspace-route-helpers";
import type { ToolPolicy } from "@/types/toolpolicy";

export const { GET, POST } = createCollectionRoutes<ToolPolicy>({
  kind: "ToolPolicy",
  plural: CRD_TOOL_POLICIES,
  errorLabel: "tool policies",
});

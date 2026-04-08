/**
 * API route for a specific workspace-scoped tool policy.
 *
 * GET    /api/workspaces/:name/toolpolicies/:policyName - Get tool policy details
 * PUT    /api/workspaces/:name/toolpolicies/:policyName - Update tool policy
 * DELETE /api/workspaces/:name/toolpolicies/:policyName - Delete tool policy
 *
 * Protected by workspace access checks.
 */

import { createItemRoutes } from "@/lib/api/crd-route-factory";
import { CRD_TOOL_POLICIES } from "@/lib/k8s/workspace-route-helpers";
import type { ToolPolicy } from "@/types/toolpolicy";

export const { GET, PUT, DELETE } = createItemRoutes<ToolPolicy>({
  kind: "ToolPolicy",
  plural: CRD_TOOL_POLICIES,
  resourceLabel: "Tool policy",
  paramKey: "policyName",
  errorLabel: "tool policy",
});

/**
 * API route for a specific workspace-scoped agent policy.
 *
 * GET    /api/workspaces/:name/agentpolicies/:policyName - Get agent policy details
 * PUT    /api/workspaces/:name/agentpolicies/:policyName - Update agent policy
 * DELETE /api/workspaces/:name/agentpolicies/:policyName - Delete agent policy
 *
 * Protected by workspace access checks.
 */

import { createItemRoutes } from "@/lib/api/crd-route-factory";
import { CRD_AGENT_POLICIES } from "@/lib/k8s/workspace-route-helpers";
import type { AgentPolicy } from "@/types/agentpolicy";

export const { GET, PUT, DELETE } = createItemRoutes<AgentPolicy>({
  kind: "AgentPolicy",
  plural: CRD_AGENT_POLICIES,
  resourceLabel: "Agent policy",
  paramKey: "policyName",
  errorLabel: "agent policy",
});

/**
 * API route for workspace-scoped agent policies.
 *
 * GET  /api/workspaces/:name/agentpolicies - List agent policies in workspace
 * POST /api/workspaces/:name/agentpolicies - Create an agent policy in workspace
 */

import { createCollectionRoutes } from "@/lib/api/crd-route-factory";
import { CRD_AGENT_POLICIES } from "@/lib/k8s/workspace-route-helpers";
import type { AgentPolicy } from "@/types/agentpolicy";

export const { GET, POST } = createCollectionRoutes<AgentPolicy>({
  kind: "AgentPolicy",
  plural: CRD_AGENT_POLICIES,
  errorLabel: "agent policies",
});

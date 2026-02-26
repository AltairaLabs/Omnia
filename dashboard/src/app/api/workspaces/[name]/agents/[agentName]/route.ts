/**
 * API routes for individual workspace agent operations.
 *
 * GET /api/workspaces/:name/agents/:agentName - Get agent details
 * PUT /api/workspaces/:name/agents/:agentName - Update agent
 * DELETE /api/workspaces/:name/agents/:agentName - Delete agent
 *
 * Protected by workspace access checks.
 */

import { createItemRoutes } from "@/lib/api/crd-route-factory";
import { CRD_AGENTS } from "@/lib/k8s/workspace-route-helpers";
import type { AgentRuntime } from "@/lib/data/types";

export const { GET, PUT, DELETE } = createItemRoutes<AgentRuntime>({
  kind: "AgentRuntime",
  plural: CRD_AGENTS,
  resourceLabel: "Agent",
  paramKey: "agentName",
  errorLabel: "this agent",
});

/**
 * API routes for workspace agents (AgentRuntime CRDs).
 *
 * GET /api/workspaces/:name/agents - List agents in workspace
 * POST /api/workspaces/:name/agents - Create a new agent
 *
 * Protected by workspace access checks.
 * These routes always talk to the real K8s API - mock data is handled
 * by the MockDataService on the client side (in demo mode).
 */

import { createCollectionRoutes } from "@/lib/api/crd-route-factory";
import { CRD_AGENTS } from "@/lib/k8s/workspace-route-helpers";
import type { AgentRuntime } from "@/lib/data/types";

export const { GET, POST } = createCollectionRoutes<AgentRuntime>({
  kind: "AgentRuntime",
  plural: CRD_AGENTS,
  errorLabel: "agents",
});

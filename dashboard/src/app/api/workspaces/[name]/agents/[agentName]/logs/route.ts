/**
 * API route for getting agent logs.
 *
 * GET /api/workspaces/:name/agents/:agentName/logs - Get logs for agent pods
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { getPodLogs } from "@/lib/k8s/crd-operations";
import { getWorkspaceResource, handleK8sError, CRD_AGENTS } from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { AgentRuntime } from "@/lib/data/types";

type RouteParams = { name: string; agentName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    try {
      const { name, agentName } = await context.params;
      const result = await getWorkspaceResource<AgentRuntime>(name, access.role!, CRD_AGENTS, agentName, "Agent");
      if (!result.ok) return result.response;

      const { searchParams } = new URL(request.url);
      const lines = parseInt(searchParams.get("lines") || "100", 10);

      const logs = await getPodLogs(result.clientOptions, `app.kubernetes.io/name=${agentName}`, lines);

      return NextResponse.json(logs);
    } catch (error) {
      return handleK8sError(error, "access agent logs");
    }
  }
);

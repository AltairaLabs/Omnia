/**
 * API route for scaling an agent.
 *
 * PUT /api/workspaces/:name/agents/:agentName/scale - Scale agent replicas
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { patchCrd } from "@/lib/k8s/crd-operations";
import { getWorkspaceResource, handleK8sError, CRD_AGENTS } from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { AgentRuntime } from "@/lib/data/types";

type RouteParams = { name: string; agentName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

export const PUT = withWorkspaceAccess<RouteParams>(
  "editor",
  async (
    request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    try {
      const { name, agentName } = await context.params;

      const body = await request.json();
      if (typeof body.replicas !== "number" || body.replicas < 0) {
        return NextResponse.json(
          { error: "Bad Request", message: "replicas must be a non-negative number" },
          { status: 400 }
        );
      }

      const result = await getWorkspaceResource<AgentRuntime>(name, access.role!, CRD_AGENTS, agentName, "Agent");
      if (!result.ok) return result.response;

      const patched = await patchCrd<AgentRuntime>(
        result.clientOptions,
        CRD_AGENTS,
        agentName,
        { spec: { replicas: body.replicas } }
      );

      return NextResponse.json(patched);
    } catch (error) {
      return handleK8sError(error, "scale this agent");
    }
  }
);

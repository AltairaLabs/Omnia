/**
 * API route for updating agent eval configuration.
 *
 * PUT /api/workspaces/:name/agents/:agentName/evals - Update eval config
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

interface EvalConfigBody {
  enabled?: boolean;
  sampling?: {
    defaultRate?: number;
    extendedRate?: number;
  };
}

function isValidRate(value: unknown): boolean {
  return typeof value === "number" && value >= 0 && value <= 100;
}

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

      const body: EvalConfigBody = await request.json();

      if (body.sampling) {
        if (body.sampling.defaultRate !== undefined && !isValidRate(body.sampling.defaultRate)) {
          return NextResponse.json(
            { error: "Bad Request", message: "sampling.defaultRate must be a number between 0 and 100" },
            { status: 400 }
          );
        }
        if (body.sampling.extendedRate !== undefined && !isValidRate(body.sampling.extendedRate)) {
          return NextResponse.json(
            { error: "Bad Request", message: "sampling.extendedRate must be a number between 0 and 100" },
            { status: 400 }
          );
        }
      }

      const result = await getWorkspaceResource<AgentRuntime>(name, access.role!, CRD_AGENTS, agentName, "Agent");
      if (!result.ok) return result.response;

      const patch: Record<string, unknown> = {
        spec: {
          evals: {
            ...(body.enabled !== undefined && { enabled: body.enabled }),
            ...(body.sampling && { sampling: body.sampling }),
          },
        },
      };

      const patched = await patchCrd<AgentRuntime>(
        result.clientOptions,
        CRD_AGENTS,
        agentName,
        patch
      );

      return NextResponse.json(patched);
    } catch (error) {
      return handleK8sError(error, "update eval configuration");
    }
  }
);

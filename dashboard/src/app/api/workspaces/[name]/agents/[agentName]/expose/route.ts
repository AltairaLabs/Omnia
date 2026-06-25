/**
 * API route for toggling an agent's external exposure (#1611).
 *
 * PATCH /api/workspaces/:name/agents/:agentName/expose
 *   body: { enabled: boolean, host?: string }
 *
 * Sets spec.facade.expose so the operator (when a default-exposure Gateway is
 * configured, #1553) provisions/removes the agent's HTTPRoute. Exposure does NOT
 * add auth — spec.externalAuth is still the gate. Protected by editor access.
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

export const PATCH = withWorkspaceAccess<RouteParams>(
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
      if (typeof body.enabled !== "boolean") {
        return NextResponse.json(
          { error: "Bad Request", message: "enabled must be a boolean" },
          { status: 400 }
        );
      }
      if (body.host !== undefined && typeof body.host !== "string") {
        return NextResponse.json(
          { error: "Bad Request", message: "host must be a string" },
          { status: 400 }
        );
      }

      const result = await getWorkspaceResource<AgentRuntime>(
        name,
        access.role!,
        CRD_AGENTS,
        agentName,
        "Agent"
      );
      if (!result.ok) return result.response;

      // host: a trimmed value sets the override; empty/absent clears it (a
      // merge-patch null removes the key).
      const host = typeof body.host === "string" ? body.host.trim() : "";
      const patched = await patchCrd<AgentRuntime>(
        result.clientOptions,
        CRD_AGENTS,
        agentName,
        { spec: { facade: { expose: { enabled: body.enabled, host: host || null } } } }
      );

      return NextResponse.json(patched);
    } catch (error) {
      return handleK8sError(error, "update external exposure for this agent");
    }
  }
);

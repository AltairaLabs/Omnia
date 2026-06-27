/**
 * API route for toggling an agent's external exposure (#1611).
 *
 * PATCH /api/workspaces/:name/agents/:agentName/expose
 *   body: { enabled: boolean, host?: string }
 *
 * Sets expose on the agent's primary facade (facades[].expose, #1576) so the
 * operator (when a default-exposure Gateway is configured, #1553) provisions/
 * removes the agent's HTTPRoute. Exposure does NOT add auth — spec.externalAuth
 * is still the gate. Protected by editor access.
 *
 * The merge-patch content type replaces a list wholesale, so we read the current
 * facades, set expose on the primary entry, and PATCH the whole facades array.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { patchCrd } from "@/lib/k8s/crd-operations";
import { getWorkspaceResource, handleK8sError, CRD_AGENTS } from "@/lib/k8s/workspace-route-helpers";
import { primaryFacade } from "@/types/agent-runtime";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { AgentRuntime } from "@/lib/data/types";

type RouteParams = { name: string; agentName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const BAD_REQUEST = "Bad Request";

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
          { error: BAD_REQUEST, message: "enabled must be a boolean" },
          { status: 400 }
        );
      }
      if (body.host !== undefined && typeof body.host !== "string") {
        return NextResponse.json(
          { error: BAD_REQUEST, message: "host must be a string" },
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

      // host: a trimmed value sets the override; empty/absent clears it.
      const host = typeof body.host === "string" ? body.host.trim() : "";
      const expose = host
        ? { enabled: body.enabled, host }
        : { enabled: body.enabled };

      // Set expose on the primary facade. A merge-patch replaces the list
      // wholesale, so rebuild the full facades array with the chosen entry
      // updated and PATCH the whole list.
      const facades = result.resource.spec.facades ?? [];
      const target = primaryFacade(result.resource.spec);
      if (!target) {
        return NextResponse.json(
          { error: BAD_REQUEST, message: "agent has no facades to expose" },
          { status: 400 }
        );
      }
      const nextFacades = facades.map(f => (f === target ? { ...f, expose } : f));

      const patched = await patchCrd<AgentRuntime>(
        result.clientOptions,
        CRD_AGENTS,
        agentName,
        { spec: { facades: nextFacades } }
      );

      return NextResponse.json(patched);
    } catch (error) {
      return handleK8sError(error, "update external exposure for this agent");
    }
  }
);

/**
 * API route for getting agent events.
 *
 * GET /api/workspaces/:name/agents/:agentName/events - Get K8s events for agent
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { getResourceEvents } from "@/lib/k8s/crd-operations";
import { getWorkspaceResource, handleK8sError, CRD_AGENTS } from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { AgentRuntime, K8sEvent } from "@/lib/data/types";

type RouteParams = { name: string; agentName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    try {
      const { name, agentName } = await context.params;
      const result = await getWorkspaceResource<AgentRuntime>(name, access.role!, CRD_AGENTS, agentName, "Agent");
      if (!result.ok) return result.response;

      const events = await getResourceEvents(result.clientOptions, "AgentRuntime", agentName);

      const k8sEvents: K8sEvent[] = events.map((e) => ({
        type: e.type,
        reason: e.reason,
        message: e.message,
        firstTimestamp: e.firstTimestamp,
        lastTimestamp: e.lastTimestamp,
        count: e.count,
        source: e.source,
        involvedObject: e.involvedObject,
      }));

      k8sEvents.sort((a, b) => new Date(b.lastTimestamp).getTime() - new Date(a.lastTimestamp).getTime());

      return NextResponse.json(k8sEvents);
    } catch (error) {
      return handleK8sError(error, "access agent events");
    }
  }
);

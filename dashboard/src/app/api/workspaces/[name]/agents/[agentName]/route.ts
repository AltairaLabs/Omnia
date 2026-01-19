/**
 * API routes for individual workspace agent operations.
 *
 * GET /api/workspaces/:name/agents/:agentName - Get agent details
 * PUT /api/workspaces/:name/agents/:agentName - Update agent
 * DELETE /api/workspaces/:name/agents/:agentName - Delete agent
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { updateCrd, deleteCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  getWorkspaceResource,
  handleK8sError,
  WORKSPACE_LABEL,
  CRD_AGENTS,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { AgentRuntime } from "@/lib/data/types";

type RouteParams = { name: string; agentName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const CRD_KIND = "AgentRuntime";

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, agentName } = await context.params;
    let auditCtx;

    try {
      const result = await getWorkspaceResource<AgentRuntime>(name, access.role!, CRD_AGENTS, agentName, "Agent");
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      auditSuccess(auditCtx, "get", agentName);
      return NextResponse.json(result.resource);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", agentName, error, 500);
      }
      return handleK8sError(error, "access this agent");
    }
  }
);

export const PUT = withWorkspaceAccess<RouteParams>(
  "editor",
  async (
    request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, agentName } = await context.params;
    let auditCtx;

    try {
      const result = await getWorkspaceResource<AgentRuntime>(name, access.role!, CRD_AGENTS, agentName, "Agent");
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      const body = await request.json();
      const updated: AgentRuntime = {
        ...result.resource,
        metadata: {
          ...result.resource.metadata,
          labels: { ...result.resource.metadata?.labels, ...body.metadata?.labels, [WORKSPACE_LABEL]: name },
          annotations: { ...result.resource.metadata?.annotations, ...body.metadata?.annotations },
        },
        spec: body.spec || result.resource.spec,
      };

      const saved = await updateCrd<AgentRuntime>(result.clientOptions, CRD_AGENTS, agentName, updated);

      auditSuccess(auditCtx, "update", agentName);
      return NextResponse.json(saved);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "update", agentName, error, 500);
      }
      return handleK8sError(error, "update this agent");
    }
  }
);

export const DELETE = withWorkspaceAccess<RouteParams>(
  "editor",
  async (
    _request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, agentName } = await context.params;
    let auditCtx;

    try {
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      await deleteCrd(result.clientOptions, CRD_AGENTS, agentName);

      auditSuccess(auditCtx, "delete", agentName);
      return new NextResponse(null, { status: 204 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "delete", agentName, error, 500);
      }
      return handleK8sError(error, "delete this agent");
    }
  }
);

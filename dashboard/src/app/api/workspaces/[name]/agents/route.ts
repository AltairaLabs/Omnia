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

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { listCrd, createCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  serverErrorResponse,
  buildCrdResource,
  CRD_AGENTS,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { AgentRuntime } from "@/lib/data/types";

const CRD_KIND = "AgentRuntime";

export const GET = withWorkspaceAccess(
  "viewer",
  async (
    _request: NextRequest,
    context: WorkspaceRouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;
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

      const agents = await listCrd<AgentRuntime>(result.clientOptions, CRD_AGENTS);

      auditSuccess(auditCtx, "list", undefined, { count: agents.length });
      return NextResponse.json(agents);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "list", undefined, error, 500);
      }
      return serverErrorResponse(error, "Failed to list agents");
    }
  }
);

export const POST = withWorkspaceAccess(
  "editor",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    let auditCtx;
    let resourceName = "";

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

      const body = await request.json();
      resourceName = body.metadata?.name || body.name || "";

      const agent = buildCrdResource(
        CRD_KIND,
        name,
        result.workspace.spec.namespace.name,
        resourceName,
        body.spec,
        body.metadata?.labels,
        body.metadata?.annotations
      );

      const created = await createCrd<AgentRuntime>(result.clientOptions, CRD_AGENTS, agent as unknown as AgentRuntime);

      auditSuccess(auditCtx, "create", resourceName);
      return NextResponse.json(created, { status: 201 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", resourceName, error, 500);
      }
      return serverErrorResponse(error, "Failed to create agent");
    }
  }
);

/**
 * API routes for workspace agents (AgentRuntime CRDs).
 *
 * GET /api/workspaces/:name/agents - List agents in workspace
 * POST /api/workspaces/:name/agents - Create a new agent
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { listCrd, createCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  serverErrorResponse,
  buildCrdResource,
  CRD_AGENTS,
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
    _user: User
  ): Promise<NextResponse> => {
    try {
      const { name } = await context.params;
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      const agents = await listCrd<AgentRuntime>(result.clientOptions, CRD_AGENTS);
      return NextResponse.json(agents);
    } catch (error) {
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
    _user: User
  ): Promise<NextResponse> => {
    try {
      const { name } = await context.params;
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      const body = await request.json();
      const agent = buildCrdResource(
        CRD_KIND,
        name,
        result.workspace.spec.namespace.name,
        body.metadata?.name || body.name,
        body.spec,
        body.metadata?.labels,
        body.metadata?.annotations
      );

      const created = await createCrd<AgentRuntime>(result.clientOptions, CRD_AGENTS, agent as AgentRuntime);
      return NextResponse.json(created, { status: 201 });
    } catch (error) {
      return serverErrorResponse(error, "Failed to create agent");
    }
  }
);

/**
 * API routes for workspace-scoped providers.
 *
 * GET /api/workspaces/:name/providers - List providers in workspace
 * POST /api/workspaces/:name/providers - Create a new provider
 *
 * Providers can be workspace-scoped (in workspace namespace) or
 * shared (in omnia-system namespace). This endpoint returns workspace-scoped ones.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { listCrd, createCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  serverErrorResponse,
  buildCrdResource,
  CRD_PROVIDERS,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { Provider } from "@/lib/data/types";

const CRD_KIND = "Provider";

/**
 * GET /api/workspaces/:name/providers
 *
 * List all providers in the workspace namespace.
 * Requires viewer role.
 */
export const GET = withWorkspaceAccess(
  "viewer",
  async (
    _request: NextRequest,
    context: WorkspaceRouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name: workspaceName } = await context.params;
    let auditCtx;

    try {
      const validation = await validateWorkspace(workspaceName, access.role!);
      if (!validation.ok) return validation.response;

      auditCtx = createAuditContext(
        workspaceName,
        validation.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      const providers = await listCrd<Provider>(
        validation.clientOptions,
        CRD_PROVIDERS
      );

      auditSuccess(auditCtx, "list", undefined, { count: providers.length });
      return NextResponse.json(providers);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "list", undefined, error, 500);
      }
      return serverErrorResponse(error, "Failed to list providers");
    }
  }
);

/**
 * POST /api/workspaces/:name/providers
 *
 * Create a new provider in the workspace namespace.
 * Requires editor role.
 */
export const POST = withWorkspaceAccess(
  "editor",
  async (
    request: NextRequest,
    context: WorkspaceRouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name: workspaceName } = await context.params;
    let auditCtx;
    let resourceName = "";

    try {
      const validation = await validateWorkspace(workspaceName, access.role!);
      if (!validation.ok) return validation.response;

      auditCtx = createAuditContext(
        workspaceName,
        validation.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      const body = await request.json();
      resourceName = body.metadata?.name || body.name || "";

      const provider = buildCrdResource(
        CRD_KIND,
        workspaceName,
        validation.workspace.spec.namespace.name,
        resourceName,
        body.spec,
        body.metadata?.labels,
        body.metadata?.annotations
      );

      const created = await createCrd<Provider>(
        validation.clientOptions,
        CRD_PROVIDERS,
        provider as unknown as Provider
      );

      auditSuccess(auditCtx, "create", resourceName);
      return NextResponse.json(created, { status: 201 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", resourceName, error, 500);
      }
      return serverErrorResponse(error, "Failed to create provider");
    }
  }
);

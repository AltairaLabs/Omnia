/**
 * API routes for a specific workspace-scoped provider.
 *
 * GET /api/workspaces/:name/providers/:providerName - Get provider details
 * PUT /api/workspaces/:name/providers/:providerName - Update provider
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { updateCrd } from "@/lib/k8s/crd-operations";
import {
  getWorkspaceResource,
  handleK8sError,
  WORKSPACE_LABEL,
  CRD_PROVIDERS,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { Provider } from "@/lib/data/types";

type RouteParams = { name: string; providerName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const CRD_KIND = "Provider";

/**
 * GET /api/workspaces/:name/providers/:providerName
 *
 * Get a specific provider in the workspace namespace.
 * Requires viewer role.
 */
export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name: workspaceName, providerName } = await context.params;
    let auditCtx;

    try {
      const result = await getWorkspaceResource<Provider>(
        workspaceName,
        access.role!,
        CRD_PROVIDERS,
        providerName,
        "Provider"
      );
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        workspaceName,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      auditSuccess(auditCtx, "get", providerName);
      return NextResponse.json(result.resource);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", providerName, error, 500);
      }
      return handleK8sError(error, "get provider");
    }
  }
);

/**
 * PUT /api/workspaces/:name/providers/:providerName
 *
 * Update a provider in the workspace namespace.
 * Requires editor role.
 */
export const PUT = withWorkspaceAccess<RouteParams>(
  "editor",
  async (
    request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name: workspaceName, providerName } = await context.params;
    let auditCtx;

    try {
      const result = await getWorkspaceResource<Provider>(
        workspaceName,
        access.role!,
        CRD_PROVIDERS,
        providerName,
        "Provider"
      );
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        workspaceName,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      const body = await request.json();
      const updated: Provider = {
        ...result.resource,
        metadata: {
          ...result.resource.metadata,
          labels: {
            ...result.resource.metadata?.labels,
            ...body.metadata?.labels,
            [WORKSPACE_LABEL]: workspaceName,
          },
          annotations: {
            ...result.resource.metadata?.annotations,
            ...body.metadata?.annotations,
          },
        },
        spec: body.spec || result.resource.spec,
      };

      const saved = await updateCrd<Provider>(
        result.clientOptions,
        CRD_PROVIDERS,
        providerName,
        updated
      );

      auditSuccess(auditCtx, "update", providerName);
      return NextResponse.json(saved);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "update", providerName, error, 500);
      }
      return handleK8sError(error, "update provider");
    }
  }
);

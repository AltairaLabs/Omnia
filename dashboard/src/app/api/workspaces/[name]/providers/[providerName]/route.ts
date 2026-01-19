/**
 * API route for a specific workspace-scoped provider.
 *
 * GET /api/workspaces/:name/providers/:providerName - Get provider details
 */

import { NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import { getCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  notFoundResponse,
  handleK8sError,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { Provider } from "@/lib/data/types";

interface RouteParams {
  name: string;
  providerName: string;
}

/**
 * GET /api/workspaces/:name/providers/:providerName
 *
 * Get a specific provider in the workspace namespace.
 * Requires viewer role.
 */
export const GET = withWorkspaceAccess<RouteParams>("viewer", async (req, ctx, access, user) => {
  const { name: workspaceName, providerName } = await ctx.params;

  const validation = await validateWorkspace(workspaceName, access.role!);
  if (!validation.ok) return validation.response;

  const auditCtx = createAuditContext(
    workspaceName,
    validation.workspace.spec.namespace.name,
    user,
    access.role!,
    "Provider"
  );

  try {
    const provider = await getCrd<Provider>(
      validation.clientOptions,
      "providers",
      providerName
    );

    if (!provider) {
      return notFoundResponse(`Provider not found: ${providerName}`);
    }

    auditSuccess(auditCtx, "get", providerName);
    return NextResponse.json(provider);
  } catch (error) {
    auditError(auditCtx, "get", providerName, error);
    return handleK8sError(error, "get provider");
  }
});

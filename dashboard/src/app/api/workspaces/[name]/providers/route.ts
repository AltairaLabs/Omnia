/**
 * API route for workspace-scoped providers.
 *
 * GET /api/workspaces/:name/providers - List providers in workspace
 *
 * Providers can be workspace-scoped (in workspace namespace) or
 * shared (in omnia-system namespace). This endpoint returns workspace-scoped ones.
 */

import { NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import { listCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  handleK8sError,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { Provider } from "@/lib/data/types";

/**
 * GET /api/workspaces/:name/providers
 *
 * List all providers in the workspace namespace.
 * Requires viewer role.
 */
export const GET = withWorkspaceAccess("viewer", async (req, ctx, access, user) => {
  const { name: workspaceName } = await ctx.params;

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
    const providers = await listCrd<Provider>(
      validation.clientOptions,
      "providers"
    );

    auditSuccess(auditCtx, "list");
    return NextResponse.json(providers);
  } catch (error) {
    auditError(auditCtx, "list", undefined, error);
    return handleK8sError(error, "list providers");
  }
});

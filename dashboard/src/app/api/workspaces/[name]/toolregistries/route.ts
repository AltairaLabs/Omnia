/**
 * API route for workspace-scoped tool registries.
 *
 * GET /api/workspaces/:name/toolregistries - List tool registries in workspace
 *
 * Tool registries can be workspace-scoped (in workspace namespace) or
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
import type { ToolRegistry } from "@/lib/data/types";

/**
 * GET /api/workspaces/:name/toolregistries
 *
 * List all tool registries in the workspace namespace.
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
    "ToolRegistry"
  );

  try {
    const toolRegistries = await listCrd<ToolRegistry>(
      validation.clientOptions,
      "toolregistries"
    );

    auditSuccess(auditCtx, "list");
    return NextResponse.json(toolRegistries);
  } catch (error) {
    auditError(auditCtx, "list", undefined, error);
    return handleK8sError(error, "list tool registries");
  }
});

/**
 * API route for a specific workspace-scoped tool registry.
 *
 * GET /api/workspaces/:name/toolregistries/:registryName - Get tool registry details
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
import type { ToolRegistry } from "@/lib/data/types";

interface RouteParams {
  name: string;
  registryName: string;
}

/**
 * GET /api/workspaces/:name/toolregistries/:registryName
 *
 * Get a specific tool registry in the workspace namespace.
 * Requires viewer role.
 */
export const GET = withWorkspaceAccess<RouteParams>("viewer", async (req, ctx, access, user) => {
  const { name: workspaceName, registryName } = await ctx.params;

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
    const toolRegistry = await getCrd<ToolRegistry>(
      validation.clientOptions,
      "toolregistries",
      registryName
    );

    if (!toolRegistry) {
      return notFoundResponse(`Tool registry not found: ${registryName}`);
    }

    auditSuccess(auditCtx, "get", registryName);
    return NextResponse.json(toolRegistry);
  } catch (error) {
    auditError(auditCtx, "get", registryName, error);
    return handleK8sError(error, "get tool registry");
  }
});

/**
 * API route for getting per-service logs (session-api / memory-api /
 * privacy-api pods).
 *
 * GET /api/workspaces/:name/services/:group/:service/logs - Get logs for
 * the pods backing a service-group member, or the workspace-level
 * privacy-api when :group is "__workspace__".
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { getPodLogs } from "@/lib/k8s/crd-operations";
import { validateWorkspace, handleK8sError } from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";

type RouteParams = { name: string; group: string; service: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

// Sentinel group value for services that are workspace-level rather than
// per-service-group (e.g. privacy-api).
const WORKSPACE_LEVEL_GROUP = "__workspace__";

const SERVICE_GROUP_LABEL = "omnia.altairalabs.ai/service-group";

/**
 * Builds the label selector for a service's pods. Workspace-level services
 * (group === "__workspace__") have no service-group label; everything else
 * is scoped to its group.
 */
function buildLabelSelector(group: string, service: string): string {
  const componentSelector = `app.kubernetes.io/component=${service}`;
  if (group === WORKSPACE_LEVEL_GROUP) {
    return componentSelector;
  }
  return `${componentSelector},${SERVICE_GROUP_LABEL}=${group}`;
}

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    try {
      const { name, group, service } = await context.params;
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      const { searchParams } = new URL(request.url);
      const tailLines = Number.parseInt(searchParams.get("tailLines") || searchParams.get("lines") || "100", 10);
      const sinceSeconds = searchParams.get("sinceSeconds")
        ? Number.parseInt(searchParams.get("sinceSeconds")!, 10)
        : undefined;

      const logs = await getPodLogs(
        result.clientOptions,
        buildLabelSelector(group, service),
        tailLines,
        sinceSeconds
      );

      return NextResponse.json({ logs });
    } catch (error) {
      return handleK8sError(error, "access service logs");
    }
  }
);

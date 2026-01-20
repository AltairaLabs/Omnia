/**
 * API route for listing scenarios in an Arena config.
 *
 * GET /api/workspaces/:name/arena/configs/:configName/scenarios - List scenarios
 *
 * Returns the scenarios discovered from the config's source.
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import {
  getWorkspaceResource,
  handleK8sError,
  CRD_ARENA_CONFIGS,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaConfig } from "@/types/arena";

type RouteParams = { name: string; configName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const CRD_KIND = "ArenaConfig";

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, configName } = await context.params;
    let auditCtx;

    try {
      const result = await getWorkspaceResource<ArenaConfig>(
        name,
        access.role!,
        CRD_ARENA_CONFIGS,
        configName,
        "Arena config"
      );
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      // Scenarios are not stored in the config status directly.
      // They need to be fetched from the source artifact.
      // For now, return the scenario count as a placeholder.
      // TODO: Implement scenario fetching from source artifact
      const scenarioCount = result.resource.status?.scenarioCount || 0;
      const scenarios: unknown[] = [];

      auditSuccess(auditCtx, "get", configName, { subresource: "scenarios", count: scenarioCount });
      return NextResponse.json(scenarios);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", configName, error, 500);
      }
      return handleK8sError(error, "list scenarios for this arena config");
    }
  }
);

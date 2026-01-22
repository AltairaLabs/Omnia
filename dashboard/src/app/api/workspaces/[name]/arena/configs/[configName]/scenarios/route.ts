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
  CRD_ARENA_SOURCES,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import { getConfigMapContent } from "@/lib/k8s/crd-operations";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaConfig, ArenaSource, Scenario } from "@/types/arena";

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

      // Get the source reference from the config
      const sourceRef = result.resource.spec?.sourceRef?.name;
      if (!sourceRef) {
        auditSuccess(auditCtx, "get", configName, { subresource: "scenarios", count: 0 });
        return NextResponse.json([]);
      }

      // Fetch the ArenaSource to get the ConfigMap reference
      const sourceResult = await getWorkspaceResource<ArenaSource>(
        name,
        access.role!,
        CRD_ARENA_SOURCES,
        sourceRef,
        "Arena source"
      );
      if (!sourceResult.ok) {
        auditSuccess(auditCtx, "get", configName, { subresource: "scenarios", count: 0 });
        return NextResponse.json([]);
      }

      // For ConfigMap sources, fetch the pack content
      const configMapName = sourceResult.resource.spec?.configMap?.name;
      if (!configMapName) {
        auditSuccess(auditCtx, "get", configName, { subresource: "scenarios", count: 0 });
        return NextResponse.json([]);
      }

      const configMapData = await getConfigMapContent(sourceResult.clientOptions, configMapName);
      if (!configMapData) {
        auditSuccess(auditCtx, "get", configName, { subresource: "scenarios", count: 0 });
        return NextResponse.json([]);
      }

      // Parse the pack content to extract scenarios
      const contentKey = Object.keys(configMapData).find(
        (key) => key.endsWith(".json") || key.endsWith(".yaml") || key.endsWith(".yml")
      );
      if (!contentKey) {
        auditSuccess(auditCtx, "get", configName, { subresource: "scenarios", count: 0 });
        return NextResponse.json([]);
      }

      let packContent: { scenarios?: Record<string, Scenario> };
      try {
        if (contentKey.endsWith(".json")) {
          packContent = JSON.parse(configMapData[contentKey]);
        } else {
          const yaml = await import("js-yaml");
          packContent = yaml.load(configMapData[contentKey]) as typeof packContent;
        }
      } catch {
        auditSuccess(auditCtx, "get", configName, { subresource: "scenarios", count: 0 });
        return NextResponse.json([]);
      }

      // Convert scenarios object to array with proper mapping
      interface PackScenario {
        id?: string;
        name?: string;
        description?: string;
        prompt_ref?: string;
        variables?: Record<string, unknown>;
        assertions?: Array<{ type: string; value: string; case_insensitive?: boolean }>;
      }

      const scenarios: Scenario[] = packContent.scenarios
        ? Object.entries(packContent.scenarios as Record<string, PackScenario>).map(([id, scenario]) => ({
            name: id,
            displayName: scenario.name || id,
            description: scenario.description,
            path: `scenarios/${id}`,
            assertions: scenario.assertions?.map(a => `${a.type}: ${a.value}`) || [],
          }))
        : [];

      auditSuccess(auditCtx, "get", configName, { subresource: "scenarios", count: scenarios.length });
      return NextResponse.json(scenarios);
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "get", configName, error, 500);
      }
      return handleK8sError(error, "list scenarios for this arena config");
    }
  }
);

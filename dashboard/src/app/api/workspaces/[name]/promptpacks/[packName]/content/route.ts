/**
 * API route for getting prompt pack content.
 *
 * GET /api/workspaces/:name/promptpacks/:packName/content - Get resolved prompt pack content
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { getConfigMapContent } from "@/lib/k8s/crd-operations";
import {
  getWorkspaceResource,
  notFoundResponse,
  handleK8sError,
  serverErrorResponse,
  CRD_PROMPTPACKS,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { PromptPack, PromptPackContent } from "@/lib/data/types";

type RouteParams = { name: string; packName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

export const GET = withWorkspaceAccess<RouteParams>(
  "viewer",
  async (
    _request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    _user: User
  ): Promise<NextResponse> => {
    try {
      const { name, packName } = await context.params;
      const result = await getWorkspaceResource<PromptPack>(name, access.role!, CRD_PROMPTPACKS, packName, "Prompt pack");
      if (!result.ok) return result.response;

      const configMapName = result.resource.spec?.source?.configMapRef?.name;
      if (!configMapName) return notFoundResponse("Prompt pack has no ConfigMap source");

      const configMapData = await getConfigMapContent(result.clientOptions, configMapName);
      if (!configMapData) return notFoundResponse(`ConfigMap not found: ${configMapName}`);

      const contentKey = Object.keys(configMapData).find(
        (key) => key.endsWith(".yaml") || key.endsWith(".yml") || key.endsWith(".json") || key === "content" || key === "pack"
      );
      if (!contentKey) return notFoundResponse("No content found in ConfigMap");

      const rawContent = configMapData[contentKey];

      let content: PromptPackContent;
      try {
        if (contentKey.endsWith(".json")) {
          content = JSON.parse(rawContent) as PromptPackContent;
        } else {
          const yaml = await import("js-yaml");
          content = yaml.load(rawContent) as PromptPackContent;
        }
      } catch (parseError) {
        return serverErrorResponse(parseError, "Failed to parse prompt pack content");
      }

      return NextResponse.json(content);
    } catch (error) {
      return handleK8sError(error, "access prompt pack content");
    }
  }
);

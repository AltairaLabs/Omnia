/**
 * API routes for workspace prompt packs (PromptPack CRDs).
 *
 * GET  /api/workspaces/:name/promptpacks - List prompt packs in workspace
 * POST /api/workspaces/:name/promptpacks - Create a new prompt pack (optionally with ConfigMap)
 *
 * The POST handler accepts an optional `content` field. When present, a backing
 * ConfigMap is created/updated and the PromptPack spec.source is set to reference it.
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { createCollectionRoutes } from "@/lib/api/crd-route-factory";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { createCrd, createOrUpdateConfigMap } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  serverErrorResponse,
  buildCrdResource,
  CRD_PROMPTPACKS,
  WORKSPACE_LABEL,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { PromptPack } from "@/lib/data/types";

// GET uses the standard factory handler
const collectionRoutes = createCollectionRoutes<PromptPack>({
  kind: "PromptPack",
  plural: CRD_PROMPTPACKS,
  errorLabel: "prompt packs",
});

export const GET = collectionRoutes.GET;

// Custom POST that optionally creates a ConfigMap alongside the PromptPack
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
      const result = await validateWorkspace(workspaceName, access.role!);
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        workspaceName,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        "PromptPack"
      );

      const body = await request.json();
      resourceName = body.metadata?.name || body.name || "";
      const content: Record<string, string> | undefined = body.content;

      // If content is provided, create/update the backing ConfigMap
      let spec = body.spec || {};
      if (content) {
        const configMapName = `${resourceName}-content`;
        await createOrUpdateConfigMap(
          result.clientOptions,
          configMapName,
          content,
          {
            [WORKSPACE_LABEL]: workspaceName,
            "omnia.altairalabs.ai/managed-by": "promptpack",
            "omnia.altairalabs.ai/promptpack": resourceName,
          }
        );
        // Override source to point to the ConfigMap
        spec = {
          ...spec,
          source: { type: "configmap", configMapRef: { name: configMapName } },
        };
      }

      const resource = buildCrdResource(
        "PromptPack",
        workspaceName,
        result.workspace.spec.namespace.name,
        resourceName,
        spec,
        body.metadata?.labels,
        body.metadata?.annotations
      );

      const created = await createCrd<PromptPack>(
        result.clientOptions,
        CRD_PROMPTPACKS,
        resource as unknown as PromptPack
      );

      auditSuccess(auditCtx, "create", resourceName);
      return NextResponse.json(created, { status: 201 });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "create", resourceName, error, 500);
      }
      return serverErrorResponse(error, "Failed to create prompt pack");
    }
  }
);

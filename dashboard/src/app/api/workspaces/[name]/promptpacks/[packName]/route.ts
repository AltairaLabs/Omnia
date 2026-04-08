/**
 * API routes for individual workspace prompt pack operations.
 *
 * GET    /api/workspaces/:name/promptpacks/:packName - Get prompt pack details
 * PUT    /api/workspaces/:name/promptpacks/:packName - Update prompt pack (optionally with ConfigMap)
 * DELETE /api/workspaces/:name/promptpacks/:packName - Delete prompt pack (and backing ConfigMap)
 *
 * The PUT handler accepts an optional `content` field. When present, the backing
 * ConfigMap is created/updated and the PromptPack spec.source is set to reference it.
 *
 * The DELETE handler automatically removes the `${packName}-content` ConfigMap if it
 * exists (no-op if it was never created).
 *
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { createItemRoutes } from "@/lib/api/crd-route-factory";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import { updateCrd, deleteCrd, createOrUpdateConfigMap, deleteConfigMap } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  getWorkspaceResource,
  handleK8sError,
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
const itemRoutes = createItemRoutes<PromptPack>({
  kind: "PromptPack",
  plural: CRD_PROMPTPACKS,
  resourceLabel: "Prompt pack",
  paramKey: "packName",
  errorLabel: "this prompt pack",
});

export const GET = itemRoutes.GET;

type ItemParams = { name: string; packName: string };

// Custom PUT with optional ConfigMap update
export const PUT = withWorkspaceAccess<ItemParams>(
  "editor",
  async (
    request: NextRequest,
    context: { params: Promise<ItemParams> },
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const params = await context.params;
    const workspaceName = params.name;
    const packName = params.packName;
    let auditCtx;

    try {
      const result = await getWorkspaceResource<PromptPack>(
        workspaceName,
        access.role!,
        CRD_PROMPTPACKS,
        packName,
        "Prompt pack"
      );
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        workspaceName,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        "PromptPack"
      );

      const body = await request.json();
      const content: Record<string, string> | undefined = body.content;

      // Validate content size before creating ConfigMap
      const MAX_CONFIGMAP_SIZE = 900 * 1024; // 900KB (under K8s 1MiB limit)
      if (content) {
        const contentSize = Object.entries(content).reduce(
          (sum, [k, v]) => sum + k.length + v.length, 0
        );
        if (contentSize > MAX_CONFIGMAP_SIZE) {
          return NextResponse.json(
            { error: "Content exceeds maximum size of 900KB" },
            { status: 413 }
          );
        }
      }

      let spec = body.spec || result.resource.spec;
      if (content) {
        const configMapName = `${packName}-content`;
        await createOrUpdateConfigMap(
          result.clientOptions,
          configMapName,
          content,
          {
            [WORKSPACE_LABEL]: workspaceName,
            "omnia.altairalabs.ai/managed-by": "promptpack",
            "omnia.altairalabs.ai/promptpack": packName,
          }
        );
        spec = {
          ...spec,
          source: { type: "configmap", configMapRef: { name: configMapName } },
        };
      }

      const updated = {
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
        spec,
      };

      const saved = await updateCrd<PromptPack>(result.clientOptions, CRD_PROMPTPACKS, packName, updated);
      auditSuccess(auditCtx, "update", packName);
      return NextResponse.json(saved);
    } catch (error) {
      if (auditCtx) auditError(auditCtx, "update", packName, error, 500);
      return handleK8sError(error, "update prompt pack");
    }
  }
);

// Custom DELETE that also cleans up the backing ConfigMap
export const DELETE = withWorkspaceAccess<ItemParams>(
  "editor",
  async (
    _request: NextRequest,
    context: { params: Promise<ItemParams> },
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const params = await context.params;
    const workspaceName = params.name;
    const packName = params.packName;
    let auditCtx;

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

      // Delete the backing ConfigMap (no-op if it doesn't exist or isn't managed by promptpack)
      await deleteConfigMap(result.clientOptions, `${packName}-content`, "promptpack");

      // Delete the PromptPack CRD
      await deleteCrd(result.clientOptions, CRD_PROMPTPACKS, packName);

      auditSuccess(auditCtx, "delete", packName);
      return new NextResponse(null, { status: 204 });
    } catch (error) {
      if (auditCtx) auditError(auditCtx, "delete", packName, error, 500);
      return handleK8sError(error, "delete prompt pack");
    }
  }
);

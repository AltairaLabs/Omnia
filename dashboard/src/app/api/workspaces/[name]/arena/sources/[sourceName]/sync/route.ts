/**
 * API route for triggering Arena source synchronization.
 *
 * POST /api/workspaces/:name/arena/sources/:sourceName/sync - Trigger sync
 *
 * Triggers a sync by updating the reconcile annotation on the source.
 * Protected by workspace access checks.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess, type WorkspaceRouteContext } from "@/lib/auth/workspace-guard";
import { patchCrd } from "@/lib/k8s/crd-operations";
import {
  getWorkspaceResource,
  handleK8sError,
  CRD_ARENA_SOURCES,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { ArenaSource } from "@/types/arena";

type RouteParams = { name: string; sourceName: string };
type RouteContext = WorkspaceRouteContext<RouteParams>;

const CRD_KIND = "ArenaSource";
const RECONCILE_ANNOTATION = "omnia.altairalabs.ai/reconcile-at";

export const POST = withWorkspaceAccess<RouteParams>(
  "editor",
  async (
    _request: NextRequest,
    context: RouteContext,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name, sourceName } = await context.params;
    let auditCtx;

    try {
      const result = await getWorkspaceResource<ArenaSource>(
        name,
        access.role!,
        CRD_ARENA_SOURCES,
        sourceName,
        "Arena source"
      );
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        CRD_KIND
      );

      // Trigger reconciliation by setting annotation with current timestamp
      const patch = {
        metadata: {
          annotations: {
            [RECONCILE_ANNOTATION]: new Date().toISOString(),
          },
        },
      };

      await patchCrd(result.clientOptions, CRD_ARENA_SOURCES, sourceName, patch);

      auditSuccess(auditCtx, "patch", sourceName, { action: "sync" });
      return NextResponse.json({ message: "Sync triggered", sourceName });
    } catch (error) {
      if (auditCtx) {
        auditError(auditCtx, "patch", sourceName, error, 500);
      }
      return handleK8sError(error, "trigger sync for this arena source");
    }
  }
);

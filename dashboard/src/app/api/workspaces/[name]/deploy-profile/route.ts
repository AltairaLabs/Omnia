/**
 * Deploy profile discovery endpoint.
 *
 * GET /api/workspaces/:name/deploy-profile
 *
 * Returns the connection details + a discovery menu (Providers with roles,
 * SkillSources) for bootstrapping the promptarena-deploy-omnia adapter config.
 * Discovery only — never returns a secret. Part of the deploy adapter API
 * surface (see api/openapi/openapi.yaml). Issue #1519.
 */

import { NextRequest, NextResponse } from "next/server";
import { withWorkspaceAccess } from "@/lib/auth/workspace-guard";
import {
  validateWorkspace,
  handleK8sError,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import { buildDeployProfile, resolveApiEndpoint } from "@/lib/data/deploy-profile";

const RESOURCE_TYPE = "DeployProfile";

interface RouteParams {
  params: Promise<{ name: string }>;
}

export const GET = withWorkspaceAccess<{ name: string }>(
  "viewer",
  async (
    request: NextRequest,
    context: RouteParams,
    access: WorkspaceAccess,
    user: User
  ): Promise<NextResponse> => {
    const { name } = await context.params;
    let auditCtx;
    try {
      const result = await validateWorkspace(name, access.role!);
      if (!result.ok) return result.response;

      auditCtx = createAuditContext(
        name,
        result.workspace.spec.namespace.name,
        user,
        access.role!,
        RESOURCE_TYPE
      );

      const profile = await buildDeployProfile(
        result.clientOptions,
        name,
        resolveApiEndpoint(request)
      );

      auditSuccess(auditCtx, "get", name, {
        providerCount: profile.providers.length,
        skillCount: profile.skills.length,
      });
      return NextResponse.json(profile);
    } catch (error) {
      if (auditCtx) auditError(auditCtx, "get", name, error, 500);
      return handleK8sError(error, "get deploy profile");
    }
  }
);

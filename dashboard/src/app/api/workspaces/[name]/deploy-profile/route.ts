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
import { listCrd } from "@/lib/k8s/crd-operations";
import {
  validateWorkspace,
  handleK8sError,
  CRD_PROVIDERS,
  CRD_SKILL_SOURCES,
  createAuditContext,
  auditSuccess,
  auditError,
} from "@/lib/k8s/workspace-route-helpers";
import type { WorkspaceAccess } from "@/types/workspace";
import type { User } from "@/lib/auth/types";
import type { Provider } from "@/lib/data/types";
import type { SkillSource } from "@/types/skill-source";
import type {
  DeployProfile,
  DeployProfileProvider,
  DeployProfileSkill,
} from "@/types/deploy-profile";

const RESOURCE_TYPE = "DeployProfile";
const DEFAULT_ROLE = "llm";

interface RouteParams {
  params: Promise<{ name: string }>;
}

/** Derive the external API endpoint from forwarded headers, env fallback. */
function resolveApiEndpoint(request: NextRequest): string {
  const host = request.headers.get("x-forwarded-host");
  if (host) {
    const proto = request.headers.get("x-forwarded-proto") || "https";
    return `${proto}://${host}`;
  }
  return process.env.OMNIA_DASHBOARD_EXTERNAL_URL || "";
}

const PHASE_READY = "Ready";

/**
 * Only Ready resources belong in a deploy profile: a deployment that
 * references a Provider/SkillSource that isn't Ready (Unavailable, Error,
 * still syncing) fails. Filtering here keeps the discovery menu to things the
 * agent can actually bind to. (#1519)
 */
function isProviderReady(p: Provider): boolean {
  return p.status?.phase === PHASE_READY;
}

function isSkillReady(s: SkillSource): boolean {
  return s.status?.phase === PHASE_READY;
}

function toProfileProvider(p: Provider): DeployProfileProvider {
  const out: DeployProfileProvider = {
    name: p.metadata.name,
    role: p.spec?.role || DEFAULT_ROLE,
    type: p.spec?.type,
  };
  if (p.spec?.model) out.model = p.spec.model;
  return out;
}

function toProfileSkill(s: SkillSource): DeployProfileSkill {
  return { name: s.metadata.name, type: s.spec.type };
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

      const [providers, skills] = await Promise.all([
        listCrd<Provider>(result.clientOptions, CRD_PROVIDERS),
        listCrd<SkillSource>(result.clientOptions, CRD_SKILL_SOURCES),
      ]);

      const profile: DeployProfile = {
        api_endpoint: resolveApiEndpoint(request),
        workspace: name,
        providers: providers.filter(isProviderReady).map(toProfileProvider),
        skills: skills.filter(isSkillReady).map(toProfileSkill),
      };

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

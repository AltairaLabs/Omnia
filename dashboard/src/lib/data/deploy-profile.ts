/** Deploy profile assembly — shared by the discovery route and CLI exchange. */
import type { NextRequest } from "next/server";
import { listCrd } from "@/lib/k8s/crd-operations";
import { CRD_PROVIDERS, CRD_SKILL_SOURCES } from "@/lib/k8s/workspace-route-helpers";
import type { Provider } from "@/lib/data/types";
import type { SkillSource } from "@/types/skill-source";
import type {
  DeployProfile,
  DeployProfileProvider,
  DeployProfileSkill,
} from "@/types/deploy-profile";
import { SUPPORTED_DEPLOY_INTENT_VERSIONS } from "@/lib/deploy/intent-versions";

const DEFAULT_ROLE = "llm";
const PHASE_READY = "Ready";

function isProviderReady(p: Provider): boolean {
  return p.status?.phase === PHASE_READY;
}
// Only llm-role providers are deployable into an AgentRuntime's spec.providers.
// embedding/tts/stt/image providers are workspace-level (memory-api) or unsupported
// as per-agent extras — bundling them breaks pack-open at first request (#1596).
function isLlmProvider(p: Provider): boolean {
  return (p.spec?.role || DEFAULT_ROLE) === DEFAULT_ROLE;
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

/** Derive the external API endpoint from forwarded headers, env fallback. */
export function resolveApiEndpoint(request: NextRequest): string {
  const host = request.headers.get("x-forwarded-host");
  if (host) {
    const proto = request.headers.get("x-forwarded-proto") || "https";
    return `${proto}://${host}`;
  }
  return process.env.OMNIA_DASHBOARD_EXTERNAL_URL || "";
}

/** Assemble the deploy profile for a workspace: Ready llm-role providers (the
 * only ones deployable into spec.providers) + Ready skills. (#1519, #1596) */
export async function buildDeployProfile(
  clientOptions: Parameters<typeof listCrd>[0],
  name: string,
  apiEndpoint: string
): Promise<DeployProfile> {
  const [providers, skills] = await Promise.all([
    listCrd<Provider>(clientOptions, CRD_PROVIDERS),
    listCrd<SkillSource>(clientOptions, CRD_SKILL_SOURCES),
  ]);
  return {
    api_endpoint: apiEndpoint,
    workspace: name,
    providers: providers
      .filter(isProviderReady)
      .filter(isLlmProvider)
      .map(toProfileProvider),
    skills: skills.filter(isSkillReady).map(toProfileSkill),
    supportedDeployIntentVersions: [...SUPPORTED_DEPLOY_INTENT_VERSIONS],
  };
}

/**
 * Assemble the promptarena-deploy-omnia `config:` block from a discovery
 * profile + a token. Pure function — no I/O. Issue #1519.
 *
 * The caller passes the providers/skills the user selected (a subset of the
 * Ready discovery menu) and which provider is the primary LLM; this shapes
 * them into the adapter's config schema.
 */

import * as yaml from "js-yaml";
import type { DeployProfile } from "@/types/deploy-profile";

const DEFAULT_BINDING = "default";
const ROLE_LLM = "llm";

interface DeployConfigBlock {
  config: {
    api_endpoint: string;
    workspace: string;
    api_token: string;
    providers: { name: string; ref: string; role: string }[];
    skills: { source: string }[];
  };
}

/**
 * Resolve which Provider CRD becomes the `default` binding (the AgentRuntime's
 * primary LLM — a deployment with no provider in the "default" group fails).
 * Prefers the caller's explicit choice (when it's an LLM in the profile), then
 * a provider already named "default", then the first LLM. Returns undefined
 * when there are no LLM providers — nothing sensible to default.
 */
function resolveDefaultProvider(
  profile: DeployProfile,
  chosen?: string
): string | undefined {
  const llms = profile.providers
    .filter((p) => p.role === ROLE_LLM)
    .map((p) => p.name);
  if (chosen && llms.includes(chosen)) return chosen;
  if (llms.includes(DEFAULT_BINDING)) return DEFAULT_BINDING;
  return llms[0];
}

export function assembleDeployConfig(
  profile: DeployProfile,
  apiToken: string,
  defaultProvider?: string
): { yaml: string; json: string } {
  const primary = resolveDefaultProvider(profile, defaultProvider);
  const block: DeployConfigBlock = {
    config: {
      api_endpoint: profile.api_endpoint,
      workspace: profile.workspace,
      api_token: apiToken,
      // Only llm-role providers are deployable into spec.providers; drop any
      // embedding/tts/stt/image provider that reached here so it can never break
      // pack-open (#1596 — defence in depth; discovery already filters these).
      // Mark the chosen LLM as the "default" binding (the runtime's primary);
      // every other provider keeps its CRD name. `ref` is always the real
      // Provider CRD name. (#1519)
      providers: profile.providers
        .filter((p) => p.role === ROLE_LLM)
        .map((p) => ({
          name: p.name === primary ? DEFAULT_BINDING : p.name,
          ref: p.name,
          role: p.role,
        })),
      // The adapter models `skills` as SkillBinding objects ({source}), not
      // bare names — must match internal/omnia/config.go's schema (#1519).
      skills: profile.skills.map((s) => ({ source: s.name })),
    },
  };
  return {
    yaml: yaml.dump(block, { lineWidth: -1, noRefs: true }),
    json: JSON.stringify(block, null, 2),
  };
}

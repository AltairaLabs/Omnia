/**
 * Assemble the promptarena-deploy-omnia `config:` block from a discovery
 * profile + a freshly minted token. Pure function — no I/O. Issue #1519.
 */

import * as yaml from "js-yaml";
import type { DeployProfile } from "@/types/deploy-profile";

interface DeployConfigBlock {
  config: {
    api_endpoint: string;
    workspace: string;
    api_token: string;
    providers: { name: string; ref: string; role: string }[];
    skills: string[];
  };
}

export function assembleDeployConfig(
  profile: DeployProfile,
  apiToken: string
): { yaml: string; json: string } {
  const block: DeployConfigBlock = {
    config: {
      api_endpoint: profile.api_endpoint,
      workspace: profile.workspace,
      api_token: apiToken,
      providers: profile.providers.map((p) => ({
        name: p.name,
        ref: p.name,
        role: p.role,
      })),
      skills: profile.skills.map((s) => s.name),
    },
  };
  return {
    yaml: yaml.dump(block, { lineWidth: -1, noRefs: true }),
    json: JSON.stringify(block, null, 2),
  };
}

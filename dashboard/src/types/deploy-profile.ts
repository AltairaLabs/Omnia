// Deploy profile discovery payload returned by
// GET /api/workspaces/{name}/deploy-profile. Raw discovery only — no secret.
// See issue #1519.

export interface DeployProfileProvider {
  /** Real Provider CRD metadata.name — used as the adapter `ref`. */
  name: string;
  /** Provider role; defaults to "llm" when the CRD omits spec.role. */
  role: string;
  /** Provider vendor/wire protocol (spec.type). */
  type: string;
  /** Model identifier (spec.model), if set. */
  model?: string;
}

export interface DeployProfileSkill {
  /** Real SkillSource CRD metadata.name. */
  name: string;
  /** SkillSource type: git | oci | configmap. */
  type: string;
}

export interface DeployProfile {
  /** External dashboard ingress URL the adapter calls. */
  api_endpoint: string;
  /** Workspace name. */
  workspace: string;
  /** Providers discovered in the workspace (the discovery menu). */
  providers: DeployProfileProvider[];
  /** SkillSources discovered in the workspace. */
  skills: DeployProfileSkill[];
  /** DeployIntent contract versions the operator accepts (highest wins). */
  supportedDeployIntentVersions: string[];
}

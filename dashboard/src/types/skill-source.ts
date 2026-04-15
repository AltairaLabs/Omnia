// SkillSource CRD types — matches api/v1alpha1/skillsource_types.go.
// Issue #829.

import type { ObjectMeta, Condition } from "./common";

export type SkillSourceType = "git" | "oci" | "configmap";

export type SkillSourcePhase =
  | "Pending"
  | "Initializing"
  | "Ready"
  | "Fetching"
  | "Error";

export interface GitSourceRef {
  url: string;
  path?: string;
  ref?: {
    branch?: string;
    tag?: string;
    commit?: string;
  };
  secretRef?: { name: string };
}

export interface OCISourceRef {
  url: string;
  insecure?: boolean;
  secretRef?: { name: string };
}

export interface ConfigMapSourceRef {
  name: string;
  key?: string;
}

export interface SkillFilter {
  include?: string[];
  exclude?: string[];
  names?: string[];
}

export interface SkillSourceSpec {
  type: SkillSourceType;
  git?: GitSourceRef;
  oci?: OCISourceRef;
  configMap?: ConfigMapSourceRef;
  interval: string;
  timeout?: string;
  suspend?: boolean;
  targetPath?: string;
  filter?: SkillFilter;
  createVersionOnSync?: boolean;
}

export interface SkillSourceArtifact {
  revision: string;
  url?: string;
  contentPath?: string;
  version?: string;
  checksum?: string;
  size?: number;
  lastUpdateTime?: string;
}

export interface SkillSourceStatus {
  phase?: SkillSourcePhase;
  observedGeneration?: number;
  artifact?: SkillSourceArtifact;
  skillCount?: number;
  conditions?: Condition[];
  lastFetchTime?: string;
  nextFetchTime?: string;
}

export interface SkillSource {
  apiVersion: "omnia.altairalabs.ai/v1alpha1";
  kind: "SkillSource";
  metadata: ObjectMeta;
  spec: SkillSourceSpec;
  status?: SkillSourceStatus;
}

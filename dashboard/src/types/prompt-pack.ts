// PromptPack CRD types - matches api/v1alpha1/promptpack_types.go

import { ObjectMeta, Condition, LocalObjectReference } from "./common";

// Enums
export type PromptPackPhase = "Pending" | "Active" | "Superseded" | "Failed";
export type PromptPackSourceType = "configmap";

export interface PromptPackSource {
  type: PromptPackSourceType;
  configMapRef?: LocalObjectReference;
}

// A reference from a PromptPack to skills synced by a SkillSource CRD.
// Matches api/v1alpha1 SkillRef.
export interface SkillRef {
  source: string;      // SkillSource CRD name (same namespace)
  include?: string[];  // frontmatter names to narrow scope (empty = all)
  mountAs?: string;    // virtual mount path rename (default = source basename)
}

// Spec
export interface PromptPackSpec {
  source: PromptPackSource;
  version: string;
  skills?: SkillRef[];
}

// Status
export interface PromptPackStatus {
  phase?: PromptPackPhase;
  activeVersion?: string;
  lastUpdated?: string;
  conditions?: Condition[];
}

// Full resource
export interface PromptPack {
  apiVersion: "omnia.altairalabs.ai/v1alpha1";
  kind: "PromptPack";
  metadata: ObjectMeta;
  spec: PromptPackSpec;
  status?: PromptPackStatus;
}

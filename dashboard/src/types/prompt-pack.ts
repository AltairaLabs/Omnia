// PromptPack CRD types - matches api/v1alpha1/promptpack_types.go

import { ObjectMeta, Condition, LocalObjectReference } from "./common";

// Enums
export type PromptPackPhase = "Pending" | "Active" | "Superseded" | "Failed";
export type PromptPackSourceType = "configmap";

export interface PromptPackSource {
  type: PromptPackSourceType;
  configMapRef?: LocalObjectReference;
}

// Spec
export interface PromptPackSpec {
  source: PromptPackSource;
  version: string;
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

// PromptPack CRD types - matches api/v1alpha1/promptpack_types.go

import { ObjectMeta, Condition, LocalObjectReference } from "./common";

// Enums
export type PromptPackPhase = "Pending" | "Active" | "Canary" | "Superseded" | "Failed";
export type RolloutStrategyType = "immediate" | "canary";
export type PromptPackSourceType = "configmap";

// Nested types
export interface CanaryConfig {
  weight: number;
  stepWeight?: number;
  interval?: string;
}

export interface RolloutStrategy {
  type: RolloutStrategyType;
  canary?: CanaryConfig;
}

export interface PromptPackSource {
  type: PromptPackSourceType;
  configMapRef?: LocalObjectReference;
}

// Spec
export interface PromptPackSpec {
  source: PromptPackSource;
  version: string;
  rollout: RolloutStrategy;
}

// Status
export interface PromptPackStatus {
  phase?: PromptPackPhase;
  activeVersion?: string;
  canaryVersion?: string;
  canaryWeight?: number;
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

// AgentRuntime CRD types - matches api/v1alpha1/agentruntime_types.go

import { ObjectMeta, Condition, LocalObjectReference, SecretKeyRef } from "./common";

// Enums
export type AgentRuntimePhase = "Pending" | "Running" | "Failed";
export type FacadeType = "websocket" | "grpc";
export type HandlerMode = "echo" | "demo" | "runtime";
export type SessionStoreType = "memory" | "redis" | "postgres";
export type ProviderType = "auto" | "claude" | "openai" | "gemini";
export type AutoscalerType = "hpa" | "keda";
export type FrameworkType = "promptkit" | "langchain" | "crewai" | "autogen" | "custom";

// Nested types
export interface PromptPackRef {
  name: string;
  version?: string;
  track?: string;
}

export interface FacadeConfig {
  type: FacadeType;
  port?: number;
  handler?: HandlerMode;
}

export interface ToolRegistryRef {
  name: string;
  namespace?: string;
}

export interface SessionConfig {
  type: SessionStoreType;
  storeRef?: LocalObjectReference;
  ttl?: string;
}

export interface KEDAConfig {
  pollingInterval?: number;
  cooldownPeriod?: number;
  triggers?: KEDATrigger[];
}

export interface KEDATrigger {
  type: string;
  metadata: Record<string, string>;
}

export interface AutoscalingConfig {
  enabled: boolean;
  type?: AutoscalerType;
  minReplicas?: number;
  maxReplicas?: number;
  targetMemoryUtilizationPercentage?: number;
  targetCPUUtilizationPercentage?: number;
  scaleDownStabilizationSeconds?: number;
  keda?: KEDAConfig;
}

export interface ResourceRequirements {
  limits?: Record<string, string>;
  requests?: Record<string, string>;
}

export interface RuntimeConfig {
  replicas?: number;
  autoscaling?: AutoscalingConfig;
  resources?: ResourceRequirements;
}

export interface ProviderDefaults {
  temperature?: number;
  topP?: number;
  maxTokens?: number;
}

export interface ProviderPricing {
  inputTokensPer1M?: string;
  outputTokensPer1M?: string;
  cacheReadPer1M?: string;
}

export interface ProviderConfig {
  type: ProviderType;
  model?: string;
  secretRef?: SecretKeyRef;
  baseURL?: string;
  config?: ProviderDefaults;
  pricing?: ProviderPricing;
}

export interface ProviderRef {
  name: string;
  namespace?: string;
}

export interface FrameworkConfig {
  type: FrameworkType;
  version?: string;
  image?: string;
}

// Spec
export interface AgentRuntimeSpec {
  framework?: FrameworkConfig;
  promptPackRef: PromptPackRef;
  facade: FacadeConfig;
  toolRegistryRef?: ToolRegistryRef;
  session?: SessionConfig;
  runtime?: RuntimeConfig;
  provider?: ProviderConfig;
  providerRef?: ProviderRef;
}

// Status
export interface ReplicaStatus {
  desired: number;
  ready: number;
  available: number;
}

export interface AgentRuntimeStatus {
  phase?: AgentRuntimePhase;
  replicas?: ReplicaStatus;
  activeVersion?: string;
  conditions?: Condition[];
  observedGeneration?: number;
}

// Full resource
export interface AgentRuntime {
  apiVersion: "omnia.altairalabs.ai/v1alpha1";
  kind: "AgentRuntime";
  metadata: ObjectMeta;
  spec: AgentRuntimeSpec;
  status?: AgentRuntimeStatus;
}

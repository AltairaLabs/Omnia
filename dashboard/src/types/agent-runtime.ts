// AgentRuntime CRD types - matches api/v1alpha1/agentruntime_types.go

import { ObjectMeta, Condition, LocalObjectReference, SecretKeyRef } from "./common";

// Enums
export type AgentRuntimePhase = "Pending" | "Running" | "Failed";
export type FacadeType = "websocket" | "grpc" | "a2a" | "rest";
export type HandlerMode = "echo" | "demo" | "runtime";
export type SessionStoreType = "memory" | "redis" | "postgres";
export type ProviderType = "claude" | "openai" | "gemini" | "ollama" | "mock";
export type AutoscalerType = "hpa" | "keda";
export type FrameworkType = "promptkit" | "langchain" | "autogen" | "custom";

// Nested types
export interface PromptPackRef {
  name: string;
  version?: string;
}

export interface MCPConfig {
  enabled?: boolean;
  port?: number;
}

export interface FacadeConfig {
  type: FacadeType;
  port?: number;
  handler?: HandlerMode;
  mcp?: MCPConfig;
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

export interface MemoryConfig {
  /** Enabled controls whether cross-session memory is active.
   * Memory is disabled by default. */
  enabled?: boolean;
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
  credential?: {
    secretRef?: SecretKeyRef;
    envVar?: string;
    filePath?: string;
  };
  baseURL?: string;
  config?: ProviderDefaults;
  pricing?: ProviderPricing;
}

export interface ProviderRef {
  name: string;
  namespace?: string;
}

export interface NamedProviderRef {
  name: string;
  providerRef: ProviderRef;
}

export interface FrameworkConfig {
  type: FrameworkType;
  version?: string;
  image?: string;
}

/** Width and height in pixels */
export interface Dimensions {
  width: number;
  height: number;
}

/** Compression guidance for images */
export type CompressionGuidance = "none" | "lossless" | "lossy-high" | "lossy-medium" | "lossy-low";

/** Video processing mode */
export type VideoProcessingMode = "frames" | "transcription" | "both" | "native";

/** Requirements for image media */
export interface ImageRequirements {
  /** Maximum file size in bytes for images */
  maxSizeBytes?: number;
  /** Maximum width and height - images exceeding these will need to be resized */
  maxDimensions?: Dimensions;
  /** Optimal dimensions for best results */
  recommendedDimensions?: Dimensions;
  /** Supported image formats (e.g., "png", "jpeg", "gif", "webp") */
  supportedFormats?: string[];
  /** Format that yields best results with this provider */
  preferredFormat?: string;
  /** Guidance on image compression */
  compressionGuidance?: CompressionGuidance;
}

/** Requirements for video media */
export interface VideoRequirements {
  /** Maximum video duration in seconds */
  maxDurationSeconds?: number;
  /** Whether the provider supports selecting video segments */
  supportsSegmentSelection?: boolean;
  /** How video is processed */
  processingMode?: VideoProcessingMode;
  /** Interval in seconds between extracted frames (when processingMode includes "frames") */
  frameExtractionInterval?: number;
}

/** Requirements for audio media */
export interface AudioRequirements {
  /** Maximum audio duration in seconds */
  maxDurationSeconds?: number;
  /** Optimal sample rate in Hz */
  recommendedSampleRate?: number;
  /** Number of audio channels (1 = mono, 2 = stereo) */
  channels?: number;
  /** Whether the provider supports selecting audio segments */
  supportsSegmentSelection?: boolean;
}

/** Requirements for document media */
export interface DocumentRequirements {
  /** Maximum number of pages that can be processed */
  maxPages?: number;
  /** Whether the provider supports OCR for scanned documents */
  supportsOCR?: boolean;
}

/** Provider-specific requirements for different media types */
export interface MediaRequirements {
  /** Requirements for image files */
  image?: ImageRequirements;
  /** Requirements for video files */
  video?: VideoRequirements;
  /** Requirements for audio files */
  audio?: AudioRequirements;
  /** Requirements for document files (PDFs, etc.) */
  document?: DocumentRequirements;
}

export interface ConsoleConfig {
  /** MIME types allowed for file uploads (e.g., "image/*", "application/pdf") */
  allowedAttachmentTypes?: string[];
  /** File extensions as fallback (e.g., ".png", ".pdf") */
  allowedExtensions?: string[];
  /** Maximum file size in bytes (default: 10485760 = 10MB) */
  maxFileSize?: number;
  /** Maximum number of files per message (default: 5) */
  maxFiles?: number;
  /** Provider-specific requirements for different media types */
  mediaRequirements?: MediaRequirements;
}

/** AgentRuntimeMode discriminates between the long-lived conversational
 * runtime (agent) and the one-shot structured-I/O function runtime
 * (function). Function invocations are recorded as ordinary sessions
 * tagged "function". */
export type AgentRuntimeMode = "agent" | "function";

/** Realtime duplex (voice / bidirectional streaming) configuration. */
export interface DuplexConfig {
  /** Whether duplex mode is enabled for this agent. */
  enabled?: boolean;
  /** Transport mode — "audio" (default) or future variants. */
  mode?: string;
}

// Spec
export interface AgentRuntimeSpec {
  /** mode selects the runtime shape. Defaults to "agent" when unset. */
  mode?: AgentRuntimeMode;
  framework?: FrameworkConfig;
  promptPackRef: PromptPackRef;
  facade: FacadeConfig;
  toolRegistryRef?: ToolRegistryRef;
  session?: SessionConfig;
  /** memory configures cross-session memory for this agent. */
  memory?: MemoryConfig;
  runtime?: RuntimeConfig;
  providers?: NamedProviderRef[];
  console?: ConsoleConfig;
  /** Realtime duplex (voice) configuration. When enabled, the console renders VoiceCallBar. */
  duplex?: DuplexConfig;
  evals?: EvalConfig;
  /** Progressive-delivery (canary) configuration. Present declares a rollout. */
  rollout?: RolloutConfig;
  /** inputSchema is the JSON Schema the function's request body is
   * validated against. Required when spec.mode === "function". */
  inputSchema?: Record<string, unknown>;
  /** outputSchema is the JSON Schema the function's response is
   * validated against. Required when spec.mode === "function". */
  outputSchema?: Record<string, unknown>;
}

/** isFunctionMode returns true when the runtime is declared as a
 * Function. Used by the dashboard's catalog filter and the deploy
 * wizard's conditional schema editors. */
export function isFunctionMode(spec: AgentRuntimeSpec): boolean {
  return spec.mode === "function";
}

export interface EvalConfig {
  enabled?: boolean;
  inline?: EvalPathConfig;
  worker?: EvalPathConfig;
  sampling?: {
    defaultRate?: number;
    extendedRate?: number;
  };
  rateLimit?: {
    maxConcurrentJudgeCalls?: number;
    maxEvalsPerSecond?: number;
  };
  sessionCompletion?: {
    inactivityTimeout?: string;
  };
}

export interface EvalPathConfig {
  groups?: string[];
}

// Rollout (progressive delivery)
export interface RolloutPause {
  /** Hold duration (e.g. "5m"). Omitted = pause indefinitely until promoted. */
  duration?: string;
}

export interface RolloutAnalysisStep {
  templateName: string;
}

/**
 * One ordered step in a rollout. Exactly one of setWeight/pause/analysis is set:
 * setWeight shifts canary traffic, pause holds, analysis gates on metrics.
 */
export interface RolloutStep {
  setWeight?: number;
  pause?: RolloutPause;
  analysis?: RolloutAnalysisStep;
}

export interface TrafficRoutingConfig {
  /** "mesh" | "replicaWeighted" | "external" (defaults resolve server-side). */
  mode?: string;
}

export interface RolloutConfig {
  /** Present declares an active rollout; absent/null is the idle state. */
  candidate?: Record<string, unknown>;
  steps?: RolloutStep[];
  trafficRouting?: TrafficRoutingConfig;
}

// Status
export interface ReplicaStatus {
  desired: number;
  ready: number;
  available: number;
}

/**
 * Live status of an in-progress (or just-completed) rollout, mirrored from the
 * operator's AgentRuntime `.status.rollout`. Present whenever a candidate has
 * been declared; `active: false` with a terminal message ("promoted",
 * "rolled back") after completion.
 */
export interface RolloutStatus {
  /** True while the rollout is progressing through its steps. */
  active?: boolean;
  /** Index into `spec.rollout.steps` of the step currently being evaluated. */
  currentStep?: number;
  /** Canary traffic percentage currently delivered (0-100). */
  currentWeight?: number;
  /** How traffic is split: "mesh" (Istio VS/DR), "replicaWeighted", "external". */
  trafficRoutingMode?: string;
  /**
   * Whether the delivered weight is exact. mesh/external enforce it precisely;
   * replicaWeighted approximates it via replica ratios (false).
   */
  trafficWeightEnforced?: boolean;
  /** Human-readable current state, e.g. "step 1: paused indefinitely". */
  message?: string;
  /** PromptPack version (or override identity) the candidate is running. */
  candidateVersion?: string;
  /** PromptPack version the stable track is running. */
  stableVersion?: string;
  /** RFC3339 timestamp the controller entered the current step. */
  stepStartedAt?: string;
}

export interface AgentRuntimeStatus {
  phase?: AgentRuntimePhase;
  replicas?: ReplicaStatus;
  activeVersion?: string;
  conditions?: Condition[];
  observedGeneration?: number;
  rollout?: RolloutStatus;
}

/** Get the default (or first) provider ref from an AgentRuntimeSpec */
export function getDefaultProviderRef(spec: AgentRuntimeSpec): ProviderRef | undefined {
  if (!spec.providers?.length) return undefined;
  const defaultProvider = spec.providers.find(p => p.name === "default");
  return defaultProvider?.providerRef ?? spec.providers[0].providerRef;
}

// Full resource
export interface AgentRuntime {
  apiVersion: "omnia.altairalabs.ai/v1alpha1";
  kind: "AgentRuntime";
  metadata: ObjectMeta;
  spec: AgentRuntimeSpec;
  status?: AgentRuntimeStatus;
}

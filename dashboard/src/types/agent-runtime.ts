// AgentRuntime CRD types - matches api/v1alpha1/agentruntime_types.go

import { ObjectMeta, Condition, LocalObjectReference, SecretKeyRef } from "./common";

// Enums
export type AgentRuntimePhase = "Pending" | "Running" | "Failed";
export type FacadeType = "websocket" | "a2a" | "rest" | "mcp";
export type HandlerMode = "echo" | "demo" | "runtime";
export type ContextStoreType = "memory" | "redis";
export type ProviderType = "claude" | "openai" | "gemini" | "ollama" | "mock";
export type AutoscalerType = "hpa" | "keda";
export type FrameworkType = "promptkit" | "langchain" | "custom";

// Enums
export type PromptPackTrack = "stable" | "prerelease";

// Nested types
export interface PromptPackRef {
  name: string;
  /** version pins an exact PromptPack version. Mutually exclusive with track. */
  version?: string;
  /** track follows a release channel instead of pinning a version. Mutually exclusive with version. */
  track?: PromptPackTrack;
}

export interface MCPConfig {
  enabled?: boolean;
  port?: number;
}

/** A2AConfig configures the A2A protocol surface on an a2a facade entry. */
export interface A2AConfig {
  port?: number;
  taskTTL?: string;
  conversationTTL?: string;
}

/** FacadeConfig is one single-protocol facade in spec.facades. */
export interface FacadeConfig {
  type: FacadeType;
  port?: number;
  handler?: HandlerMode;
  /** a2a configures the A2A surface; only meaningful on a type:"a2a" facade. */
  a2a?: A2AConfig;
  /** mcp configures the MCP surface; only meaningful on a type:"mcp" facade. */
  mcp?: MCPConfig;
  /** managementPlane gates this facade's internal mgmt twin (default true). */
  managementPlane?: boolean;
  /** expose opts the agent into operator-provisioned external exposure (#1553/#1611). */
  expose?: FacadeExposeConfig;
}

/** FacadeExposeConfig opts an agent into operator-provisioned external exposure. */
export interface FacadeExposeConfig {
  enabled?: boolean;
  /** host overrides the generated {name}.{namespace}.{baseDomain} hostname. */
  host?: string;
}

export interface ToolRegistryRef {
  name: string;
  namespace?: string;
}

export interface ContextConfig {
  type: ContextStoreType;
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
  /** Audio format (e.g., "pcm", "opus") */
  format?: string;
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
  /** Transport mode — "audio" (default) or "audiovideo". */
  mode?: "audio" | "audiovideo";
  /** Negotiated realtime audio format — the source of truth for the client
   * capture/playback rate (the runtime advertises it as the RuntimeHello
   * counter-offer). NOT the file-upload media requirement. */
  audio?: { recommendedSampleRate?: number; channels?: number; format?: string };
}

// Spec
/** clientKeys auth: per-caller client keys stored as Secrets, each carrying an
 * arbitrary claim map surfaced to ToolPolicy as identity.claims.*. */
export interface ClientKeysAuth {
  defaultRole?: string;
  trustEndUserHeader?: boolean;
}

/** oidc auth: OIDC JWT validation. */
export interface OidcAuth {
  issuer: string;
  audience: string;
  claimMapping?: { subject?: string; endUser?: string };
}

/** edgeTrust auth: edge-injected claim headers, no re-verification. The role
 * header is always read into identity.claims.role and is not configurable. */
export interface EdgeTrustAuth {
  headerMapping?: { subject?: string; endUser?: string; email?: string };
  claimsFromHeaders?: Record<string, string>;
}

/** externalAuth configures data-plane authentication for the agent facades.
 * The management plane is gated per-facade via facades[].managementPlane. */
export interface ExternalAuth {
  clientKeys?: ClientKeysAuth;
  oidc?: OidcAuth;
  edgeTrust?: EdgeTrustAuth;
}

export interface AgentRuntimeSpec {
  /** mode selects the runtime shape. Defaults to "agent" when unset. */
  mode?: AgentRuntimeMode;
  framework?: FrameworkConfig;
  promptPackRef: PromptPackRef;
  /** facades composes one or more single-protocol facades (#1576). */
  facades: FacadeConfig[];
  toolRegistryRef?: ToolRegistryRef;
  context?: ContextConfig;
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
  /** externalAuth configures authentication for external data-plane traffic. */
  externalAuth?: ExternalAuth;
}

/** primaryFacade selects the facade used for single-surface display purposes:
 * the websocket facade if present, else the rest facade, else the first facade.
 * Returns undefined when the agent declares no facades. */
export function primaryFacade(spec: AgentRuntimeSpec): FacadeConfig | undefined {
  return (
    spec.facades?.find(f => f.type === "websocket") ??
    spec.facades?.find(f => f.type === "rest") ??
    spec.facades?.[0]
  );
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

/** FacadeEndpoint is one externally-reachable URL derived from an HTTPRoute. */
export interface FacadeEndpoint {
  host: string;
  path: string;
  port: number;
  protocol: "websocket" | "a2a" | "mcp" | "rest";
  reason?: string;
  routeName: string;
  routeNamespace: string;
  scheme: string;
  url: string;
  valid: boolean;
}

export interface AgentRuntimeStatus {
  phase?: AgentRuntimePhase;
  replicas?: ReplicaStatus;
  activeVersion?: string;
  conditions?: Condition[];
  observedGeneration?: number;
  rollout?: RolloutStatus;
  /** facade reports externally-reachable endpoints. Empty => in-cluster only. */
  facade?: {
    endpoints?: FacadeEndpoint[];
  };
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

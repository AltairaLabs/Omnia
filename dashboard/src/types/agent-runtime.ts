// AgentRuntime CRD types - matches api/v1alpha1/agentruntime_types.go

import { ObjectMeta, Condition, LocalObjectReference, SecretKeyRef } from "./common";

// Enums
export type AgentRuntimePhase = "Pending" | "Running" | "Failed";
export type FacadeType = "websocket" | "grpc";
export type HandlerMode = "echo" | "demo" | "runtime";
export type SessionStoreType = "memory" | "redis" | "postgres";
export type ProviderType = "claude" | "openai" | "gemini" | "ollama" | "mock";
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
  console?: ConsoleConfig;
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

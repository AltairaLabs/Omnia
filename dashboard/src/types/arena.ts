/**
 * Arena Fleet CRD types - matches api/v1alpha1/arena*_types.go
 *
 * Arena Fleet provides three capabilities:
 * - Evaluation: Assess quality, correctness, and safety of AI agents
 * - Load Testing: Validate performance and scalability at enterprise scale
 * - Data Generation: Create synthetic conversations for training and testing
 */

import { ObjectMeta, Condition, LocalObjectReference } from "./common";

// =============================================================================
// ArenaSource - Where PromptKit bundles come from
// =============================================================================

export type ArenaSourceType = "configmap" | "git" | "oci" | "s3";
export type ArenaSourcePhase = "Pending" | "Ready" | "Failed";

/** Git repository reference configuration */
export interface GitRef {
  /** Branch name to track */
  branch?: string;
  /** Tag name to checkout */
  tag?: string;
  /** Specific commit SHA */
  commit?: string;
  /** SemVer constraint (e.g., ">=1.0.0 <2.0.0") */
  semver?: string;
}

/** Git source specification */
export interface GitSourceSpec {
  /** Repository URL (HTTPS or SSH) */
  url: string;
  /** Reference to checkout */
  ref?: GitRef;
  /** Subdirectory path within the repository */
  path?: string;
}

/** OCI registry reference configuration */
export interface OCIRef {
  /** Tag name */
  tag?: string;
  /** Digest (sha256:...) */
  digest?: string;
  /** SemVer constraint */
  semver?: string;
}

/** OCI registry source specification */
export interface OCISourceSpec {
  /** OCI repository URL (oci://ghcr.io/org/repo) */
  url: string;
  /** Reference to pull */
  ref?: OCIRef;
}

/** S3 bucket source specification */
export interface S3SourceSpec {
  /** S3 bucket name */
  bucket: string;
  /** Object prefix/path */
  prefix?: string;
  /** AWS region */
  region?: string;
  /** S3-compatible endpoint URL */
  endpoint?: string;
}

/** ConfigMap source specification */
export interface ConfigMapSourceSpec {
  /** ConfigMap name */
  name: string;
}

/** Artifact produced by ArenaSource reconciliation */
export interface Artifact {
  /** Revision identifier (e.g., "main@sha1:abc123" for git) */
  revision: string;
  /** Internal URL to fetch the artifact */
  url: string;
  /** SHA256 checksum of the artifact */
  checksum: string;
  /** Size in bytes */
  size?: number;
  /** When the artifact was last updated */
  lastUpdateTime: string;
}

/** ArenaSource specification */
export interface ArenaSourceSpec {
  /** Source type */
  type: ArenaSourceType;
  /** Reconciliation interval (e.g., "5m", "1h") */
  interval?: string;
  /** Git source configuration */
  git?: GitSourceSpec;
  /** OCI registry configuration */
  oci?: OCISourceSpec;
  /** S3 bucket configuration */
  s3?: S3SourceSpec;
  /** ConfigMap configuration */
  configMapRef?: ConfigMapSourceSpec;
  /** Secret containing credentials */
  secretRef?: LocalObjectReference;
  /** Suspend reconciliation */
  suspend?: boolean;
}

/** ArenaSource status */
export interface ArenaSourceStatus {
  /** Current phase */
  phase?: ArenaSourcePhase;
  /** Produced artifact */
  artifact?: Artifact;
  /** Last attempted revision */
  lastAttemptedRevision?: string;
  /** Standard conditions */
  conditions?: Condition[];
  /** Observed generation */
  observedGeneration?: number;
}

/** ArenaSource resource - defines where PromptKit bundles come from */
export interface ArenaSource {
  apiVersion: "omnia.altairalabs.ai/v1alpha1";
  kind: "ArenaSource";
  metadata: ObjectMeta;
  spec: ArenaSourceSpec;
  status?: ArenaSourceStatus;
}

// =============================================================================
// ArenaConfig - Configuration combining source with providers/tools
// =============================================================================

export type ArenaConfigPhase = "Pending" | "Ready" | "Failed";

/** Reference to another resource */
export interface ResourceRef {
  /** Resource name */
  name: string;
  /** Resource namespace (optional, defaults to same namespace) */
  namespace?: string;
}

/** Scenario filtering configuration */
export interface ScenarioFilter {
  /** Glob patterns to include (e.g., ["scenarios/*.yaml"]) */
  include?: string[];
  /** Glob patterns to exclude (e.g., ["scenarios/*-wip.yaml"]) */
  exclude?: string[];
}

/** Self-play configuration for evaluation scenarios */
export interface SelfPlayConfig {
  /** Provider for simulated user */
  providerRef?: ResourceRef;
  /** Persona filtering */
  personas?: {
    /** Glob patterns to include */
    include?: string[];
  };
}

/** Default configuration values */
export interface ArenaDefaults {
  /** Default temperature for LLM calls */
  temperature?: number;
  /** Default concurrency level */
  concurrency?: number;
  /** Default timeout per scenario */
  timeout?: string;
}

/** ArenaConfig specification */
export interface ArenaConfigSpec {
  /** Reference to the ArenaSource */
  sourceRef: ResourceRef;
  /** Path to arena.yaml within the artifact (default: "arena.yaml") */
  arenaFile?: string;
  /** Scenario filtering */
  scenarios?: ScenarioFilter;
  /** Provider references (workspace or shared) */
  providers?: ResourceRef[];
  /** ToolRegistry references (workspace or shared) */
  toolRegistries?: ResourceRef[];
  /** Self-play configuration */
  selfPlay?: SelfPlayConfig;
  /** Default values */
  defaults?: ArenaDefaults;
  /** Suspend reconciliation */
  suspend?: boolean;
}

/** Provider status in ArenaConfig */
export interface ArenaProviderStatus {
  /** Provider name */
  name: string;
  /** Provider namespace */
  namespace?: string;
  /** Status (Ready, NotFound, Error) */
  status: string;
  /** Error message if any */
  message?: string;
}

/** ToolRegistry status in ArenaConfig */
export interface ArenaToolRegistryStatus {
  /** ToolRegistry name */
  name: string;
  /** ToolRegistry namespace */
  namespace?: string;
  /** Number of tools available */
  toolCount?: number;
  /** Status (Ready, NotFound, Error) */
  status: string;
}

/** ArenaConfig status */
export interface ArenaConfigStatus {
  /** Current phase */
  phase?: ArenaConfigPhase;
  /** Source revision being used */
  sourceRevision?: string;
  /** Number of scenarios discovered */
  scenarioCount?: number;
  /** Provider statuses */
  providers?: ArenaProviderStatus[];
  /** ToolRegistry statuses */
  toolRegistries?: ArenaToolRegistryStatus[];
  /** Standard conditions */
  conditions?: Condition[];
  /** Observed generation */
  observedGeneration?: number;
}

/** ArenaConfig resource - combines source with providers/tools */
export interface ArenaConfig {
  apiVersion: "omnia.altairalabs.ai/v1alpha1";
  kind: "ArenaConfig";
  metadata: ObjectMeta;
  spec: ArenaConfigSpec;
  status?: ArenaConfigStatus;
}

// =============================================================================
// ArenaJob - Execution of evaluation/loadtest/datagen
// =============================================================================

export type ArenaJobType = "evaluation" | "loadtest" | "datagen";
export type ArenaJobPhase = "Pending" | "Running" | "Completed" | "Failed" | "Cancelled";

/** Worker autoscaling configuration */
export interface WorkerAutoscaleConfig {
  /** Enable autoscaling */
  enabled: boolean;
  /** Minimum replicas */
  min: number;
  /** Maximum replicas */
  max: number;
}

/** Worker configuration */
export interface WorkerConfig {
  /** Number of worker replicas */
  replicas?: number;
  /** Autoscaling configuration */
  autoscale?: WorkerAutoscaleConfig;
  /** Resource requirements */
  resources?: {
    limits?: Record<string, string>;
    requests?: Record<string, string>;
  };
}

/** Work queue configuration */
export interface QueueConfig {
  /** Queue type */
  type: "redis" | "nats" | "memory";
  /** Connection reference (for redis/nats) */
  connectionRef?: LocalObjectReference;
}

/** Output destination configuration */
export interface OutputConfig {
  /** Output type */
  type: "s3" | "configmap" | "pvc";
  /** S3 configuration */
  s3?: {
    bucket: string;
    prefix?: string;
    region?: string;
  };
  /** ConfigMap name for small outputs */
  configMapRef?: LocalObjectReference;
  /** PVC name for local storage */
  pvcRef?: LocalObjectReference;
}

/** Evaluation-specific configuration */
export interface EvaluationConfig {
  /** Output formats to generate */
  outputFormats?: ("json" | "junit" | "csv")[];
  /** Passing score threshold (0-1) */
  passingThreshold?: number;
  /** Continue on failure */
  continueOnFailure?: boolean;
}

/** Load test profile stage */
export interface LoadStage {
  /** Stage duration (e.g., "5m") */
  duration: string;
  /** Target virtual users at end of stage */
  targetVUs: number;
}

/** Load test threshold */
export interface LoadThreshold {
  /** Metric name */
  metric: string;
  /** Comparison operator */
  operator: "<" | ">" | "<=" | ">=" | "==" | "!=";
  /** Threshold value */
  value: string;
}

/** Load test-specific configuration */
export interface LoadTestConfig {
  /** Load profile type */
  profileType?: "constant" | "ramp" | "spike" | "soak";
  /** Load stages */
  stages?: LoadStage[];
  /** Target requests per second (alternative to stages) */
  targetRPS?: number;
  /** Test duration (for constant profile) */
  duration?: string;
  /** Performance thresholds */
  thresholds?: LoadThreshold[];
}

/** Data generation-specific configuration */
export interface DataGenConfig {
  /** Target number of samples to generate */
  sampleCount: number;
  /** Generation mode */
  mode?: "selfplay" | "template" | "variation";
  /** Output format */
  outputFormat?: "jsonl" | "parquet" | "csv";
  /** Quality filters to apply */
  filters?: string[];
  /** Deduplication enabled */
  deduplicate?: boolean;
}

/** Schedule configuration */
export interface ScheduleConfig {
  /** Cron expression */
  cron: string;
  /** Timezone (default: UTC) */
  timezone?: string;
  /** Concurrency policy */
  concurrencyPolicy?: "Allow" | "Forbid" | "Replace";
}

/** ArenaJob specification */
export interface ArenaJobSpec {
  /** Reference to ArenaConfig */
  configRef: ResourceRef;
  /** Job type */
  type: ArenaJobType;
  /** Scenario filtering (overrides config) */
  scenarios?: ScenarioFilter;
  /** Worker configuration */
  workers?: WorkerConfig;
  /** Queue configuration */
  queue?: QueueConfig;
  /** Output configuration */
  output?: OutputConfig;
  /** Evaluation-specific config */
  evaluation?: EvaluationConfig;
  /** Load test-specific config */
  loadtest?: LoadTestConfig;
  /** Data generation-specific config */
  datagen?: DataGenConfig;
  /** Schedule for recurring jobs */
  schedule?: ScheduleConfig;
  /** Job timeout */
  timeout?: string;
  /** Suspend job */
  suspend?: boolean;
}

/** Worker status within a job */
export interface JobWorkerStatus {
  /** Desired worker count */
  desired: number;
  /** Active/running workers */
  active: number;
  /** Completed workers */
  succeeded?: number;
  /** Failed workers */
  failed?: number;
}

/** ArenaJob status */
export interface ArenaJobStatus {
  /** Current phase */
  phase?: ArenaJobPhase;
  /** Job start time */
  startTime?: string;
  /** Job completion time */
  completionTime?: string;
  /** Total tasks/scenarios to process */
  totalTasks?: number;
  /** Completed tasks */
  completedTasks?: number;
  /** Failed tasks */
  failedTasks?: number;
  /** Worker status */
  workers?: JobWorkerStatus;
  /** URL to results artifact */
  resultsUrl?: string;
  /** Standard conditions */
  conditions?: Condition[];
  /** Observed generation */
  observedGeneration?: number;
}

/** ArenaJob resource - executes evaluation/loadtest/datagen */
export interface ArenaJob {
  apiVersion: "omnia.altairalabs.ai/v1alpha1";
  kind: "ArenaJob";
  metadata: ObjectMeta;
  spec: ArenaJobSpec;
  status?: ArenaJobStatus;
}

// =============================================================================
// Results Types
// =============================================================================

/** Individual evaluation result */
export interface EvaluationResult {
  /** Scenario name */
  scenario: string;
  /** Pass/fail status */
  status: "pass" | "fail" | "error" | "skipped";
  /** Score (0-1) if applicable */
  score?: number;
  /** Duration in milliseconds */
  durationMs?: number;
  /** Assertion results */
  assertions?: {
    name: string;
    passed: boolean;
    message?: string;
  }[];
  /** Error details if failed */
  error?: string;
  /** Provider used */
  provider?: string;
}

/** Aggregated evaluation results */
export interface EvaluationResults {
  /** Job name */
  jobName: string;
  /** Completion timestamp */
  completedAt: string;
  /** Summary statistics */
  summary: {
    total: number;
    passed: number;
    failed: number;
    errors: number;
    skipped: number;
    passRate: number;
    avgScore?: number;
    avgDurationMs: number;
  };
  /** Individual results */
  results: EvaluationResult[];
}

/** Load test metric data point */
export interface LoadTestDataPoint {
  /** Timestamp */
  timestamp: string;
  /** Requests per second */
  rps: number;
  /** Active virtual users */
  vus: number;
  /** Latency percentiles in milliseconds */
  latencyP50: number;
  latencyP95: number;
  latencyP99: number;
  /** Error rate (0-1) */
  errorRate: number;
  /** Tokens per second */
  tokensPerSecond?: number;
}

/** Load test results */
export interface LoadTestResults {
  /** Job name */
  jobName: string;
  /** Completion timestamp */
  completedAt: string;
  /** Summary statistics */
  summary: {
    totalRequests: number;
    totalErrors: number;
    avgRps: number;
    peakRps: number;
    avgLatencyMs: number;
    p95LatencyMs: number;
    p99LatencyMs: number;
    errorRate: number;
    thresholdsPassed: boolean;
  };
  /** Threshold results */
  thresholds?: {
    metric: string;
    passed: boolean;
    actual: string;
    expected: string;
  }[];
  /** Time series data */
  timeSeries: LoadTestDataPoint[];
}

/** Generated data sample */
export interface DataGenSample {
  /** Sample ID */
  id: string;
  /** Conversation turns */
  conversation: {
    role: "user" | "assistant" | "system";
    content: string;
  }[];
  /** Sample metadata */
  metadata?: Record<string, unknown>;
  /** Quality score */
  qualityScore?: number;
}

/** Data generation results */
export interface DataGenResults {
  /** Job name */
  jobName: string;
  /** Completion timestamp */
  completedAt: string;
  /** Summary statistics */
  summary: {
    totalGenerated: number;
    totalFiltered: number;
    totalDeduplicated: number;
    finalCount: number;
    avgQualityScore?: number;
  };
  /** Sample preview (first N samples) */
  samples: DataGenSample[];
  /** Output location */
  outputUrl: string;
}

/** Union type for all result types */
export type ArenaJobResults = EvaluationResults | LoadTestResults | DataGenResults;

// =============================================================================
// Stats Types
// =============================================================================

/** Arena statistics for dashboard overview */
export interface ArenaStats {
  sources: {
    total: number;
    ready: number;
    failed: number;
    active: number;
  };
  configs: {
    total: number;
    ready: number;
    scenarios: number;
  };
  jobs: {
    total: number;
    running: number;
    queued: number;
    completed: number;
    failed: number;
    successRate: number;
  };
}

// =============================================================================
// Scenario Types (from artifact)
// =============================================================================

/** Scenario metadata from PromptKit bundle */
export interface Scenario {
  /** Scenario name/ID */
  name: string;
  /** Display name */
  displayName?: string;
  /** Description */
  description?: string;
  /** Tags for filtering */
  tags?: string[];
  /** Expected assertions */
  assertions?: string[];
  /** File path within artifact */
  path: string;
}

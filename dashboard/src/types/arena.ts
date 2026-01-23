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
  revision?: string;
  /** Internal URL to fetch the artifact (legacy - tar.gz serving) */
  url?: string;
  /** SHA256 checksum of the artifact */
  checksum: string;
  /** Size in bytes */
  size?: number;
  /** When the artifact was last updated */
  lastUpdateTime: string;
  /** Relative path to content in workspace volume (e.g., "arena/my-source/.arena/versions/abc123") */
  contentPath?: string;
  /** Version identifier (short hash of checksum) */
  version?: string;
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
  configMap?: ConfigMapSourceSpec;
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
export type ArenaJobPhase = "Pending" | "Running" | "Succeeded" | "Failed" | "Cancelled";

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
  /** Enable verbose/debug logging for promptarena */
  verbose?: boolean;
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

/** Job progress tracking (work item level) */
export interface JobProgress {
  /** Total work items */
  total?: number;
  /** Completed work items */
  completed?: number;
  /** Failed work items */
  failed?: number;
  /** Pending work items */
  pending?: number;
}

/** Job result with summary metrics */
export interface JobResult {
  /** URL to detailed results */
  url?: string;
  /** Summary metrics (passRate, totalItems, passedItems, failedItems, avgDurationMs) */
  summary?: Record<string, string>;
}

/** ArenaJob status */
export interface ArenaJobStatus {
  /** Current phase */
  phase?: ArenaJobPhase;
  /** Job start time */
  startTime?: string;
  /** Job completion time */
  completionTime?: string;
  /** Job progress tracking */
  progress?: JobProgress;
  /** Job result summary */
  result?: JobResult;
  /** Active worker count */
  activeWorkers?: number;
  /** Worker status */
  workers?: JobWorkerStatus;
  /** Standard conditions */
  conditions?: Condition[];
  /** Observed generation */
  observedGeneration?: number;
  /** Last schedule time (for recurring jobs) */
  lastScheduleTime?: string;
  /** Next schedule time (for recurring jobs) */
  nextScheduleTime?: string;
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
// Metrics Types
// =============================================================================

/** Job metrics for real-time monitoring */
export interface ArenaJobMetrics {
  /** Progress percentage (0-100) */
  progress?: number;
  /** Current requests per second */
  currentRps?: number;
  /** Current latency percentiles */
  latencyP50?: number;
  latencyP95?: number;
  latencyP99?: number;
  /** Current error rate */
  errorRate?: number;
  /** Active workers */
  activeWorkers?: number;
  /** Tasks per second */
  tasksPerSecond?: number;
  /** Completed scenarios */
  completedScenarios?: number;
  /** Total scenarios */
  totalScenarios?: number;
}

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
  /** Reference to the prompt to use */
  promptRef?: string;
}

// =============================================================================
// Arena.yaml Content Types (parsed from ArenaSource artifact)
// =============================================================================

/** Prompt config reference in arena.yaml */
export interface ArenaPromptConfigRef {
  /** Prompt ID used for reference */
  id: string;
  /** Path to the prompt file */
  file: string;
  /** Variable overrides */
  vars?: Record<string, string>;
}

/** Provider reference in arena.yaml */
export interface ArenaProviderRef {
  /** Path to the provider file */
  file: string;
  /** Provider group (e.g., "default", "evaluation") */
  group?: string;
}

/** Scenario reference in arena.yaml */
export interface ArenaScenarioRef {
  /** Path to the scenario file */
  file: string;
}

/** Tool reference in arena.yaml */
export interface ArenaToolRef {
  /** Path to the tool file */
  file: string;
}

/** MCP server configuration */
export interface ArenaMcpServer {
  /** Command to run the MCP server */
  command: string;
  /** Arguments for the command */
  args?: string[];
  /** Environment variables */
  env?: Record<string, string>;
}

/** Judge configuration for LLM-as-judge evaluation */
export interface ArenaJudge {
  /** Provider ID to use for this judge */
  provider: string;
}

/** Judge defaults configuration */
export interface ArenaJudgeDefaults {
  /** Default prompt for judge-based assertions */
  prompt?: string;
  /** Registry path for judge resources */
  registryPath?: string;
}

/** Output configuration in defaults */
export interface ArenaOutputConfig {
  /** Output directory */
  dir?: string;
  /** Output formats to generate */
  formats?: string[];
}

/** Session recording configuration */
export interface ArenaSessionConfig {
  /** Enable session recording */
  enabled?: boolean;
  /** Directory for session files */
  dir?: string;
}

/** State management configuration */
export interface ArenaStateConfig {
  /** Enable state management */
  enabled?: boolean;
  /** Maximum history turns to retain */
  maxHistoryTurns?: number;
  /** Persistence backend */
  persistence?: string;
  /** Redis URL for persistence */
  redisUrl?: string;
}

/** Default configuration values in arena.yaml */
export interface ArenaDefaultsConfig {
  /** Default temperature for LLM calls */
  temperature?: number;
  /** Default top_p for LLM calls */
  topP?: number;
  /** Default max tokens */
  maxTokens?: number;
  /** Random seed for reproducibility */
  seed?: number;
  /** Concurrency level */
  concurrency?: number;
  /** Timeout per scenario */
  timeout?: string;
  /** Maximum retries */
  maxRetries?: number;
  /** Output configuration */
  output?: ArenaOutputConfig;
  /** Session recording configuration */
  session?: ArenaSessionConfig;
  /** Failure behavior conditions */
  failOn?: string[];
  /** State management configuration */
  state?: ArenaStateConfig;
}

/** Self-play configuration in arena.yaml */
export interface ArenaSelfPlayConfig {
  /** Enable self-play mode */
  enabled: boolean;
  /** Persona file for simulated user */
  persona?: string;
  /** Provider ID for simulated user */
  provider?: string;
}

// =============================================================================
// Parsed Content Types (fully resolved from file references)
// =============================================================================

/** Parsed prompt config from .prompt.yaml file */
export interface ParsedPromptConfig {
  /** Prompt ID */
  id: string;
  /** Human-readable name */
  name: string;
  /** Prompt version */
  version?: string;
  /** Description */
  description?: string;
  /** Task type */
  taskType?: string;
  /** System template (may be truncated for display) */
  systemTemplate?: string;
  /** Variables used in the template */
  variables?: Array<{
    name: string;
    type: string;
    required?: boolean;
    default?: string;
    description?: string;
  }>;
  /** Tool names this prompt can use */
  allowedTools?: string[];
  /** Validators configured */
  validators?: Array<{
    type: string;
    config?: Record<string, unknown>;
  }>;
  /** Source file path */
  file: string;
}

/** Parsed provider config from .provider.yaml file */
export interface ParsedProviderConfig {
  /** Provider ID */
  id: string;
  /** Provider name */
  name: string;
  /** Provider type (openai, anthropic, gemini, mock) */
  type: string;
  /** Model identifier */
  model: string;
  /** Provider group */
  group?: string;
  /** Pricing information */
  pricing?: {
    inputPer1kTokens?: number;
    outputPer1kTokens?: number;
  };
  /** Default parameters */
  defaults?: {
    temperature?: number;
    maxTokens?: number;
    topP?: number;
  };
  /** Source file path */
  file: string;
}

/** Parsed scenario from .scenario.yaml file */
export interface ParsedScenario {
  /** Scenario ID */
  id: string;
  /** Scenario name */
  name: string;
  /** Description */
  description?: string;
  /** Task type */
  taskType?: string;
  /** Conversation turns */
  turns?: Array<{
    role: "user" | "assistant";
    content: string;
  }>;
  /** Number of turns */
  turnCount?: number;
  /** Expected assertions/behaviors */
  assertions?: string[];
  /** Tags for filtering */
  tags?: string[];
  /** Source file path */
  file: string;
}

/** Parsed tool from .tool.yaml file */
export interface ParsedTool {
  /** Tool name */
  name: string;
  /** Tool description */
  description: string;
  /** Tool mode (mock, live, mcp) */
  mode?: string;
  /** Timeout in milliseconds */
  timeout?: number;
  /** Input schema (JSON Schema) */
  inputSchema?: Record<string, unknown>;
  /** Output schema (JSON Schema) */
  outputSchema?: Record<string, unknown>;
  /** Whether mock data is available */
  hasMockData?: boolean;
  /** Source file path */
  file: string;
}

/** A file in the arena package */
export interface ArenaPackageFile {
  /** File path relative to package root */
  path: string;
  /** File content (YAML/JSON as string) - only populated when fetched individually */
  content?: string;
  /** File type based on YAML kind field */
  type: "arena" | "prompt" | "provider" | "scenario" | "tool" | "persona" | "other";
  /** File size in bytes */
  size: number;
}

/** Directory structure node for file tree */
export interface ArenaPackageTreeNode {
  /** Node name (file or directory name) */
  name: string;
  /** Full path */
  path: string;
  /** Whether this is a directory */
  isDirectory: boolean;
  /** Children nodes (for directories) */
  children?: ArenaPackageTreeNode[];
  /** File type (for files) */
  type?: ArenaPackageFile["type"];
}

/** Arena pack content for file browser view */
export interface ArenaConfigContent {
  /** Package files with metadata (type, size) */
  files: ArenaPackageFile[];
  /** File tree structure for navigation */
  fileTree: ArenaPackageTreeNode[];
  /** Entry point file path (e.g., config.arena.yaml) */
  entryPoint?: string;
}

// =============================================================================
// Version Types (for ArenaSource version switching)
// =============================================================================

/** Metadata for a single version of ArenaSource content */
export interface ArenaVersion {
  /** Version hash (short hash identifier) */
  hash: string;
  /** When this version was created/synced */
  createdAt: string;
  /** Total size of the version content in bytes */
  size: number;
  /** Number of files in this version */
  fileCount: number;
  /** Whether this is the most recently synced (latest) version */
  isLatest: boolean;
}

/** Response from the versions API */
export interface ArenaVersionsResponse {
  /** Name of the ArenaSource */
  sourceName: string;
  /** Current HEAD version hash (active version) */
  head: string | null;
  /** List of available versions */
  versions: ArenaVersion[];
}

// =============================================================================
// Source Content Types (for folder browser in config creation)
// =============================================================================

/** A node in the source content tree (file or directory) */
export interface ArenaSourceContentNode {
  /** Node name (file or directory name) */
  name: string;
  /** Full path relative to source root */
  path: string;
  /** Whether this is a directory */
  isDirectory: boolean;
  /** Children nodes (for directories only) */
  children?: ArenaSourceContentNode[];
  /** File size in bytes (for files only) */
  size?: number;
}

/** Response from the source content API */
export interface ArenaSourceContentResponse {
  /** Name of the ArenaSource */
  sourceName: string;
  /** Content tree structure */
  tree: ArenaSourceContentNode[];
  /** Total number of files */
  fileCount: number;
  /** Total number of directories */
  directoryCount: number;
}

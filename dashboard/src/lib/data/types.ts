/**
 * Data service types and interface.
 *
 * This abstraction allows swapping between mock data (for demos)
 * and real K8s API data (for production) without changing
 * the consuming code.
 */

import type {
  ServerMessage,
  ConnectionStatus,
  ContentPart,
} from "@/types/websocket";
import type {
  Session as SessionType,
  SessionListOptions,
  SessionSearchOptions,
  SessionMessageOptions,
  SessionListResponse,
  SessionMessagesResponse,
} from "@/types/session";

import type { AgentRuntime as AgentRuntimeType } from "@/types/agent-runtime";
import type { PromptPack as PromptPackType } from "@/types/prompt-pack";
import type { ToolRegistry as ToolRegistryType } from "@/types/tool-registry";
import type {
  ArenaSource as ArenaSourceType,
  ArenaSourceSpec,
  ArenaJob as ArenaJobType,
  ArenaJobSpec,
  ArenaJobResults,
  ArenaStats,
} from "@/types/arena";
import type { ArenaJobListOptions, ArenaJobMetrics } from "./arena-service";

// Re-export CRD types from the centralized type definitions
export type {
  ObjectMeta,
  Condition,
} from "@/types/common";

export type {
  AgentRuntime,
  AgentRuntimeSpec,
  AgentRuntimeStatus,
  AgentRuntimePhase,
} from "@/types/agent-runtime";

export type {
  PromptPack,
  PromptPackSpec,
  PromptPackStatus,
  PromptPackPhase,
} from "@/types/prompt-pack";

export type {
  ToolRegistry,
  ToolRegistrySpec,
  ToolRegistryStatus,
  ToolRegistryPhase,
  DiscoveredTool,
} from "@/types/tool-registry";

export type {
  ArenaSource,
  ArenaSourceSpec,
  ArenaSourceStatus,
  ArenaSourcePhase,
  ArenaJob,
  ArenaJobSpec,
  ArenaJobStatus,
  ArenaJobPhase,
  ArenaJobType,
  ArenaJobResults,
  ArenaStats,
} from "@/types/arena";

export type { ArenaJobListOptions, ArenaJobMetrics } from "./arena-service";

export type {
  Session as SessionData,
  SessionSummary,
  Message as SessionMessage,
  SessionListOptions,
  SessionSearchOptions,
  SessionMessageOptions,
  SessionListResponse,
  SessionMessagesResponse,
} from "@/types/session";

// Provider types (re-exported from generated types)
import type {
  Provider as ProviderType,
  ProviderSpec as ProviderSpecType,
  ProviderStatus as ProviderStatusType,
} from "@/types/generated/provider";
export type Provider = ProviderType;
export type ProviderSpec = ProviderSpecType;
export type ProviderStatus = ProviderStatusType;
export type ProviderPhase = "Ready" | "Error";

// Log entry for agent logs
export interface LogEntry {
  timestamp: string;
  level: string;
  message: string;
  container?: string;
}

// Stats response
export interface Stats {
  agents: {
    total: number;
    running: number;
    pending: number;
    failed: number;
  };
  promptPacks: {
    total: number;
    active: number;
    canary: number;
    pending: number;
    failed: number;
  };
  tools: {
    total: number;
    available: number;
    degraded: number;
    unavailable: number;
  };
}

// PromptPack content (resolved from ConfigMap)
export interface PromptPackContent {
  id?: string;
  name?: string;
  version?: string;
  description?: string;
  template_engine?: {
    version?: string;
    syntax?: string;
  };
  prompts?: Record<string, PromptDefinition>;
  tools?: ToolDefinition[];
  fragments?: Record<string, string>;
  validators?: ValidatorDefinition[];
}

export interface PromptDefinition {
  id?: string;
  name?: string;
  version?: string;
  system_template?: string;
  variables?: PromptVariable[];
  tools?: string[];
  parameters?: Record<string, unknown>;
  validators?: string[];
}

export interface PromptVariable {
  name: string;
  type: string;
  required?: boolean;
  values?: string[];
}

export interface ToolDefinition {
  name: string;
  description?: string;
  parameters?: Record<string, unknown>;
}

export interface ValidatorDefinition {
  id: string;
  name?: string;
  description?: string;
  config?: Record<string, unknown>;
}

/**
 * Kubernetes Event (simplified for dashboard display).
 */
export interface K8sEvent {
  /** Event type: Normal or Warning */
  type: "Normal" | "Warning";
  /** Short reason string (e.g., "Scheduled", "Pulled", "Created") */
  reason: string;
  /** Human-readable message */
  message: string;
  /** First time the event occurred */
  firstTimestamp: string;
  /** Last time the event occurred */
  lastTimestamp: string;
  /** Number of times this event has occurred */
  count: number;
  /** Source component that generated the event */
  source: {
    component?: string;
    host?: string;
  };
  /** The object this event is about */
  involvedObject: {
    kind: string;
    name: string;
    namespace?: string;
  };
}

/**
 * Cost allocation item for per-agent cost tracking.
 */
export interface CostAllocationItem {
  agent: string;
  namespace: string;
  provider: string;
  model: string;
  team?: string;
  inputTokens: number;
  outputTokens: number;
  cacheHits: number;
  requests: number;
  inputCost: number;
  outputCost: number;
  cacheSavings: number;
  totalCost: number;
}

/**
 * Cost time series data point.
 * Uses dynamic provider keys instead of hardcoded providers.
 */
export interface CostTimeSeriesPoint {
  timestamp: string;
  /** Cost per provider (e.g., { anthropic: 0.5, openai: 0.3, mock: 0.1 }) */
  byProvider: Record<string, number>;
  total: number;
}

/**
 * Cost summary with totals and breakdowns.
 * Uses dynamic provider data instead of hardcoded anthropic/openai.
 */
export interface CostSummary {
  totalCost: number;
  totalInputCost: number;
  totalOutputCost: number;
  totalCacheSavings: number;
  totalRequests: number;
  totalTokens: number;
  inputTokens: number;
  outputTokens: number;
  projectedMonthlyCost: number;
  inputPercent: number;
  outputPercent: number;
}

/**
 * Provider cost breakdown.
 */
export interface ProviderCost {
  name: string;
  provider: string;
  cost: number;
  requests: number;
  tokens: number;
}

/**
 * Model cost breakdown.
 */
export interface ModelCost {
  model: string;
  displayName: string;
  provider: string;
  cost: number;
  requests: number;
  tokens: number;
}

/**
 * Complete cost data response.
 */
export interface CostData {
  /** Whether cost data is available (Prometheus is configured) */
  available: boolean;
  /** Reason for unavailability */
  reason?: string;
  /** Cost summary with totals */
  summary: CostSummary;
  /** Per-agent cost breakdown */
  byAgent: CostAllocationItem[];
  /** Per-provider cost breakdown */
  byProvider: ProviderCost[];
  /** Per-model cost breakdown */
  byModel: ModelCost[];
  /** Time series data for charts */
  timeSeries: CostTimeSeriesPoint[];
  /** URL to Grafana dashboard for detailed analysis */
  grafanaUrl?: string;
}

/**
 * Options for fetching costs.
 */
export interface CostOptions {
  namespace?: string;
}

/**
 * Options for fetching logs.
 */
export interface LogOptions {
  tailLines?: number;
  sinceSeconds?: number;
  container?: string;
}

/**
 * Data service interface.
 * Implementations provide either mock data or real API data.
 *
 * All workspace-scoped methods take a `workspace` parameter which is:
 * - In demo mode: used to filter mock data by namespace (workspace name = namespace)
 * - In live mode: used to call workspace-scoped API routes
 */
export interface DataService {
  /** Service name for debugging */
  readonly name: string;

  /** Whether this is a mock/demo service */
  readonly isDemo: boolean;

  // Agents (workspace-scoped)
  getAgents(workspace: string): Promise<AgentRuntimeType[]>;
  getAgent(workspace: string, name: string): Promise<AgentRuntimeType | undefined>;
  createAgent(workspace: string, spec: Record<string, unknown>): Promise<AgentRuntimeType>;
  scaleAgent(workspace: string, name: string, replicas: number): Promise<AgentRuntimeType>;
  getAgentLogs(workspace: string, name: string, options?: LogOptions): Promise<LogEntry[]>;
  getAgentEvents(workspace: string, name: string): Promise<K8sEvent[]>;

  // PromptPacks (workspace-scoped)
  getPromptPacks(workspace: string): Promise<PromptPackType[]>;
  getPromptPack(workspace: string, name: string): Promise<PromptPackType | undefined>;
  getPromptPackContent(workspace: string, name: string): Promise<PromptPackContent | undefined>;

  // ToolRegistries (workspace-scoped)
  getToolRegistries(workspace: string): Promise<ToolRegistryType[]>;
  getToolRegistry(workspace: string, name: string): Promise<ToolRegistryType | undefined>;

  // Providers (workspace-scoped)
  getProviders(workspace: string): Promise<Provider[]>;
  getProvider(workspace: string, name: string): Promise<Provider | undefined>;

  // Shared ToolRegistries (system-wide, in omnia-system namespace)
  getSharedToolRegistries(): Promise<ToolRegistryType[]>;
  getSharedToolRegistry(name: string): Promise<ToolRegistryType | undefined>;

  // Shared Providers (system-wide, in omnia-system namespace)
  getSharedProviders(): Promise<Provider[]>;
  getSharedProvider(name: string): Promise<Provider | undefined>;

  // Stats (workspace-scoped)
  getStats(workspace: string): Promise<Stats>;

  // Costs
  getCosts(options?: CostOptions): Promise<CostData>;

  // Arena Sources (workspace-scoped)
  getArenaSources(workspace: string): Promise<ArenaSourceType[]>;
  getArenaSource(workspace: string, name: string): Promise<ArenaSourceType | undefined>;
  createArenaSource(workspace: string, name: string, spec: ArenaSourceSpec): Promise<ArenaSourceType>;
  updateArenaSource(workspace: string, name: string, spec: ArenaSourceSpec): Promise<ArenaSourceType>;
  deleteArenaSource(workspace: string, name: string): Promise<void>;
  syncArenaSource(workspace: string, name: string): Promise<void>;

  // Arena Jobs (workspace-scoped)
  getArenaJobs(workspace: string, options?: ArenaJobListOptions): Promise<ArenaJobType[]>;
  getArenaJob(workspace: string, name: string): Promise<ArenaJobType | undefined>;
  getArenaJobResults(workspace: string, name: string): Promise<ArenaJobResults | undefined>;
  getArenaJobMetrics(workspace: string, name: string): Promise<ArenaJobMetrics | undefined>;
  createArenaJob(workspace: string, name: string, spec: ArenaJobSpec): Promise<ArenaJobType>;
  cancelArenaJob(workspace: string, name: string): Promise<void>;
  deleteArenaJob(workspace: string, name: string): Promise<void>;
  getArenaJobLogs(workspace: string, name: string, options?: LogOptions): Promise<LogEntry[]>;

  // Arena Stats (workspace-scoped)
  getArenaStats(workspace: string): Promise<ArenaStats>;

  // Sessions (workspace-scoped)
  getSessions(workspace: string, options?: SessionListOptions): Promise<SessionListResponse>;
  getSessionById(workspace: string, sessionId: string): Promise<SessionType | undefined>;
  searchSessions(workspace: string, options: SessionSearchOptions): Promise<SessionListResponse>;
  getSessionMessages(workspace: string, sessionId: string, options?: SessionMessageOptions): Promise<SessionMessagesResponse>;

  // Agent WebSocket connections (uses namespace from workspace)
  createAgentConnection(namespace: string, name: string): AgentConnection;
}

/**
 * Agent WebSocket connection interface.
 * Provides a unified interface for communicating with agents
 * whether in demo mode (mock) or production (real WebSocket).
 */
export interface AgentConnection {
  /** Connect to the agent */
  connect(): void;

  /** Disconnect from the agent */
  disconnect(): void;

  /** Send a message to the agent, optionally with multi-modal content parts */
  send(content: string, options?: { sessionId?: string; parts?: ContentPart[] }): void;

  /** Register a handler for incoming messages */
  onMessage(handler: (message: ServerMessage) => void): void;

  /** Register a handler for connection status changes */
  onStatusChange(handler: (status: ConnectionStatus, error?: string) => void): void;

  /** Get current connection status */
  getStatus(): ConnectionStatus;

  /** Get current session ID (if connected) */
  getSessionId(): string | null;

  /**
   * Get maximum payload size in bytes (from server capabilities).
   * Returns null if not connected or not yet received.
   * Files larger than this should use the upload mechanism.
   */
  getMaxPayloadSize(): number | null;
}

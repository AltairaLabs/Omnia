/**
 * Data service types and interface.
 *
 * This abstraction allows swapping between mock data (for demos)
 * and real operator API data (for production) without changing
 * the consuming code.
 */

import type { components } from "../api/schema";
import type {
  ServerMessage,
  ConnectionStatus,
  ContentPart,
} from "@/types/websocket";

// Re-export schema types for convenience
export type AgentRuntime = components["schemas"]["AgentRuntime"];
export type AgentRuntimeSpec = components["schemas"]["AgentRuntimeSpec"];
export type AgentRuntimeStatus = components["schemas"]["AgentRuntimeStatus"];
export type PromptPack = components["schemas"]["PromptPack"];
export type PromptPackSpec = components["schemas"]["PromptPackSpec"];
export type PromptPackStatus = components["schemas"]["PromptPackStatus"];
export type ToolRegistry = components["schemas"]["ToolRegistry"];
export type ToolRegistrySpec = components["schemas"]["ToolRegistrySpec"];
export type ToolRegistryStatus = components["schemas"]["ToolRegistryStatus"];
export type DiscoveredTool = components["schemas"]["DiscoveredTool"];
export type Provider = components["schemas"]["Provider"];
export type ProviderSpec = components["schemas"]["ProviderSpec"];
export type ProviderStatus = components["schemas"]["ProviderStatus"];
export type Stats = components["schemas"]["Stats"];
export type Condition = components["schemas"]["Condition"];
export type ObjectMeta = components["schemas"]["ObjectMeta"];
export type LogEntry = components["schemas"]["LogEntry"];

// Phase types for filtering
export type AgentPhase = "Pending" | "Running" | "Failed";
export type PromptPackPhase = "Pending" | "Active" | "Canary" | "Failed";
export type ToolRegistryPhase = "Pending" | "Ready" | "Degraded" | "Failed";
export type ProviderPhase = "Pending" | "Ready" | "Failed";

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
 */
export interface DataService {
  /** Service name for debugging */
  readonly name: string;

  /** Whether this is a mock/demo service */
  readonly isDemo: boolean;

  // Agents
  getAgents(namespace?: string): Promise<AgentRuntime[]>;
  getAgent(namespace: string, name: string): Promise<AgentRuntime | undefined>;
  createAgent(spec: Record<string, unknown>): Promise<AgentRuntime>;
  scaleAgent(namespace: string, name: string, replicas: number): Promise<AgentRuntime>;
  getAgentLogs(namespace: string, name: string, options?: LogOptions): Promise<LogEntry[]>;
  getAgentEvents(namespace: string, name: string): Promise<K8sEvent[]>;

  // PromptPacks
  getPromptPacks(namespace?: string): Promise<PromptPack[]>;
  getPromptPack(namespace: string, name: string): Promise<PromptPack | undefined>;
  getPromptPackContent(namespace: string, name: string): Promise<PromptPackContent | undefined>;

  // ToolRegistries
  getToolRegistries(namespace?: string): Promise<ToolRegistry[]>;
  getToolRegistry(namespace: string, name: string): Promise<ToolRegistry | undefined>;

  // Providers
  getProviders(namespace?: string): Promise<Provider[]>;
  getProvider(namespace: string, name: string): Promise<Provider | undefined>;

  // Stats & Namespaces
  getStats(): Promise<Stats>;
  getNamespaces(): Promise<string[]>;

  // Costs
  getCosts(options?: CostOptions): Promise<CostData>;

  // Agent WebSocket connections
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

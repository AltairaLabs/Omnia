/**
 * Mock data service implementation.
 * Returns sample data for demo/testing purposes.
 */

import type {
  DataService,
  AgentRuntime,
  PromptPack,
  PromptPackContent,
  ToolRegistry,
  Provider,
  Stats,
  LogEntry,
  LogOptions,
  ObjectMeta,
  AgentRuntimeSpec,
  CostData,
  CostOptions,
  K8sEvent,
  AgentConnection,
  CostAllocationItem,
  ArenaJobListOptions,
} from "./types";
import type {
  ArenaSource,
  ArenaSourceSpec,
  ArenaJob,
  ArenaJobSpec,
  ArenaJobResults,
  ArenaStats,
} from "@/types/arena";
import type { ArenaJobMetrics } from "./arena-service";

import type {
  ServerMessage,
  ConnectionStatus,
  ContentPart,
} from "@/types/websocket";

import {
  mockAgentRuntimes,
  mockPromptPacks,
  mockToolRegistries,
  getMockStats,
  mockCostAllocation,
  mockCostTimeSeries,
  getMockCostSummary,
  getMockCostByProvider,
  getMockCostByModel,
} from "../mock-data";
import { LiveAgentConnection } from "./live-service";

// Mock Arena data
const mockArenaSources: ArenaSource[] = [
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ArenaSource",
    metadata: {
      name: "customer-support-scenarios",
      namespace: "default",
      uid: "arena-source-1",
      creationTimestamp: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString(),
    },
    spec: {
      type: "configmap",
      interval: "5m",
      configMap: { name: "support-scenarios-v1" },
    },
    status: {
      phase: "Ready",
      artifact: {
        revision: "v1.0.0",
        url: "http://arena-artifacts/customer-support-scenarios/v1.0.0.tar.gz",
        checksum: "sha256:abc123",
        lastUpdateTime: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
      },
      conditions: [
        {
          type: "Ready",
          status: "True",
          lastTransitionTime: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
          reason: "Succeeded",
          message: "Artifact fetched successfully",
        },
      ],
    },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ArenaSource",
    metadata: {
      name: "sales-eval-suite",
      namespace: "default",
      uid: "arena-source-2",
      creationTimestamp: new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString(),
    },
    spec: {
      type: "configmap",
      interval: "10m",
      configMap: { name: "sales-scenarios-v2" },
    },
    status: {
      phase: "Ready",
      artifact: {
        revision: "v2.1.0",
        url: "http://arena-artifacts/sales-eval-suite/v2.1.0.tar.gz",
        checksum: "sha256:def456",
        lastUpdateTime: new Date(Date.now() - 30 * 60 * 1000).toISOString(),
      },
      conditions: [
        {
          type: "Ready",
          status: "True",
          lastTransitionTime: new Date(Date.now() - 30 * 60 * 1000).toISOString(),
          reason: "Succeeded",
          message: "Artifact fetched successfully",
        },
      ],
    },
  },
];

const mockArenaJobs: ArenaJob[] = [
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ArenaJob",
    metadata: {
      name: "support-eval-20240115",
      namespace: "default",
      uid: "arena-job-1",
      creationTimestamp: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(),
    },
    spec: {
      sourceRef: { name: "support-eval-config" },
      type: "evaluation",
      evaluation: { outputFormats: ["json", "junit"], passingThreshold: 0.8 },
    },
    status: {
      phase: "Succeeded",
      startTime: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(),
      completionTime: new Date(Date.now() - 90 * 60 * 1000).toISOString(),
      progress: {
        total: 8,
        completed: 8,
        failed: 0,
        pending: 0,
      },
      result: {
        url: "/results/support-eval-20240115",
        summary: {
          totalItems: "8",
          passedItems: "7",
          failedItems: "1",
          passRate: "87.5",
          avgDurationMs: "1500",
        },
      },
      conditions: [
        {
          type: "Complete",
          status: "True",
          lastTransitionTime: new Date(Date.now() - 90 * 60 * 1000).toISOString(),
          reason: "JobCompleted",
          message: "Job completed successfully",
        },
      ],
    },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ArenaJob",
    metadata: {
      name: "sales-eval-running",
      namespace: "default",
      uid: "arena-job-2",
      creationTimestamp: new Date(Date.now() - 30 * 60 * 1000).toISOString(),
    },
    spec: {
      sourceRef: { name: "sales-eval-config" },
      type: "evaluation",
      evaluation: { outputFormats: ["json"], passingThreshold: 0.75 },
    },
    status: {
      phase: "Running",
      startTime: new Date(Date.now() - 30 * 60 * 1000).toISOString(),
      progress: {
        total: 12,
        completed: 7,
        failed: 0,
        pending: 5,
      },
      activeWorkers: 2,
      workers: { desired: 2, active: 2 },
      conditions: [
        {
          type: "Running",
          status: "True",
          lastTransitionTime: new Date(Date.now() - 30 * 60 * 1000).toISOString(),
          reason: "JobRunning",
          message: "Job is running",
        },
      ],
    },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ArenaJob",
    metadata: {
      name: "support-eval-failed",
      namespace: "default",
      uid: "arena-job-3",
      creationTimestamp: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(),
    },
    spec: {
      sourceRef: { name: "support-eval-config" },
      type: "evaluation",
      evaluation: { outputFormats: ["json"], passingThreshold: 0.9 },
    },
    status: {
      phase: "Failed",
      startTime: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(),
      completionTime: new Date(Date.now() - 23 * 60 * 60 * 1000).toISOString(),
      progress: {
        total: 8,
        completed: 8,
        failed: 0,
        pending: 0,
      },
      result: {
        summary: {
          totalItems: "8",
          passedItems: "3",
          failedItems: "5",
          passRate: "37.5",
          avgDurationMs: "2100",
        },
      },
      conditions: [
        {
          type: "Failed",
          status: "True",
          lastTransitionTime: new Date(Date.now() - 23 * 60 * 60 * 1000).toISOString(),
          reason: "JobFailed",
          message: "Too many failed scenarios",
        },
      ],
    },
  },
];

/** Mock scenario data for pack content (kept for future use) */
const _mockScenarios = [
  { name: "greeting", displayName: "Greeting Test", description: "Tests greeting handling", tags: ["basic"] },
  { name: "refund-request", displayName: "Refund Request", description: "Tests refund handling", tags: ["support", "financial"] },
  { name: "product-inquiry", displayName: "Product Inquiry", description: "Tests product questions", tags: ["support", "sales"] },
  { name: "escalation", displayName: "Escalation Test", description: "Tests escalation flow", tags: ["support", "critical"] },
];

// Simulate network delay for realistic demo experience
function delay(ms = 100): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// Generate unique IDs
let idCounter = 0;
function generateId(): string {
  idCounter += 1;
  return `${Date.now()}-${idCounter}-${Math.random().toString(36).slice(2, 7)}`;
}

// Mock responses for demo mode
const MOCK_RESPONSES = [
  {
    content: "I'd be happy to help you with that! Let me look into it.",
    toolCalls: [
      {
        name: "search_database",
        arguments: { query: "user request" },
        result: { found: true, records: 3 },
      },
    ],
  },
  {
    content: "Based on my analysis, here's what I found:\n\n1. Your account is in good standing\n2. No recent issues detected\n3. All services are operational\n\nIs there anything specific you'd like me to help you with?",
    toolCalls: [],
  },
  {
    content: "Let me check that for you using our tools.",
    toolCalls: [
      {
        name: "get_user_info",
        arguments: { user_id: "demo-user" },
        result: { name: "Demo User", plan: "premium", created: "2024-01-15" },
      },
      {
        name: "check_permissions",
        arguments: { user_id: "demo-user", resource: "settings" },
        result: { allowed: true, roles: ["admin", "user"] },
      },
    ],
  },
];

/**
 * Mock agent connection for demo mode.
 * Simulates WebSocket communication with streaming responses.
 */
export class MockAgentConnection implements AgentConnection {
  private status: ConnectionStatus = "disconnected";
  private sessionId: string | null = null;
  private readonly messageHandlers: Array<(message: ServerMessage) => void> = [];
  private readonly statusHandlers: Array<(status: ConnectionStatus, error?: string) => void> = [];
  private mockIndex = 0;

  constructor(
    private readonly namespace: string,
    private readonly agentName: string
  ) {}

  connect(): void {
    this.setStatus("connecting");

    // Simulate connection delay
    setTimeout(() => {
      this.sessionId = `mock-session-${generateId()}`;
      this.setStatus("connected");

      // Send connected message
      this.emitMessage({
        type: "connected",
        session_id: this.sessionId,
        timestamp: new Date().toISOString(),
      });
    }, 200);
  }

  disconnect(): void {
    this.sessionId = null;
    this.setStatus("disconnected");
  }

  send(content: string, _options?: { sessionId?: string; parts?: ContentPart[] }): void {
    if (this.status !== "connected") {
      console.warn("Cannot send message: not connected");
      return;
    }

    // Simulate response (parts are ignored in mock mode)
    this.simulateResponse(content);
  }

  onMessage(handler: (message: ServerMessage) => void): void {
    this.messageHandlers.push(handler);
  }

  onStatusChange(handler: (status: ConnectionStatus, error?: string) => void): void {
    this.statusHandlers.push(handler);
  }

  getStatus(): ConnectionStatus {
    return this.status;
  }

  getSessionId(): string | null {
    return this.sessionId;
  }

  getMaxPayloadSize(): number | null {
    // Mock returns 16MB to match the default server config
    return 16 * 1024 * 1024;
  }

  private setStatus(status: ConnectionStatus, error?: string): void {
    this.status = status;
    this.statusHandlers.forEach((h) => h(status, error));
  }

  private emitMessage(message: ServerMessage): void {
    this.messageHandlers.forEach((h) => h(message));
  }

  private simulateResponse(_userMessage: string): void {
    const mockResponse = MOCK_RESPONSES[this.mockIndex % MOCK_RESPONSES.length];
    this.mockIndex++;

    // Simulate streaming response - send content word by word
    const words = mockResponse.content.split(" ");

    // Start streaming chunks
    words.forEach((word, index) => {
      setTimeout(() => {
        this.emitMessage({
          type: "chunk",
          session_id: this.sessionId || undefined,
          content: (index > 0 ? " " : "") + word,
          timestamp: new Date().toISOString(),
        });
      }, 100 + index * 50);
    });

    // Send tool calls after content
    const toolDelay = 100 + words.length * 50 + 200;
    mockResponse.toolCalls.forEach((tc, index) => {
      const toolId = `tool-${generateId()}`;

      // Tool call (pending)
      setTimeout(() => {
        this.emitMessage({
          type: "tool_call",
          session_id: this.sessionId || undefined,
          tool_call: {
            id: toolId,
            name: tc.name,
            arguments: tc.arguments,
          },
          timestamp: new Date().toISOString(),
        });
      }, toolDelay + index * 700);

      // Tool result (success)
      setTimeout(() => {
        this.emitMessage({
          type: "tool_result",
          session_id: this.sessionId || undefined,
          tool_result: {
            id: toolId,
            result: tc.result,
          },
          timestamp: new Date().toISOString(),
        });
      }, toolDelay + index * 700 + 400);
    });

    // Send done message
    const doneDelay = toolDelay + mockResponse.toolCalls.length * 700 + 500;
    setTimeout(() => {
      this.emitMessage({
        type: "done",
        session_id: this.sessionId || undefined,
        timestamp: new Date().toISOString(),
      });
    }, doneDelay);
  }
}

// Mock log message templates
const MOCK_LOG_TEMPLATES = [
  { level: "info", message: "Server started on port 8080" },
  { level: "info", message: "WebSocket connection established" },
  { level: "info", message: "Session created: sess_{id}" },
  { level: "debug", message: "Processing message from client" },
  { level: "info", message: "LLM request sent to provider" },
  { level: "debug", message: "Tokens used - input: {input}, output: {output}" },
  { level: "info", message: "Tool call: {tool}({args})" },
  { level: "info", message: "Tool response received in {ms}ms" },
  { level: "info", message: "Response streamed to client" },
  { level: "warn", message: "High latency detected: {ms}ms" },
  { level: "error", message: "Connection timeout after 30s" },
  { level: "info", message: "Session ended: sess_{id}" },
  { level: "debug", message: "Cleanup completed for session" },
  { level: "info", message: "Health check passed" },
  { level: "warn", message: "Memory usage at 75%" },
] as const;

const TOOL_NAMES = ["search_database", "get_user_info", "send_email", "fetch_data"];
const CONTAINERS = ["facade", "runtime"];

function generateMockLogs(count: number, containers: string[] = CONTAINERS): LogEntry[] {
  const logs: LogEntry[] = [];
  const now = Date.now();

  for (let i = 0; i < count; i++) {
    const template = MOCK_LOG_TEMPLATES[Math.floor(Math.random() * MOCK_LOG_TEMPLATES.length)];
    const message = template.message
      .replace("{id}", Math.random().toString(36).slice(2, 10))
      .replace("{input}", String(Math.floor(Math.random() * 2000) + 500))
      .replace("{output}", String(Math.floor(Math.random() * 1000) + 100))
      .replace("{ms}", String(Math.floor(Math.random() * 2000) + 100))
      .replace("{tool}", TOOL_NAMES[Math.floor(Math.random() * TOOL_NAMES.length)])
      .replace("{args}", '{"query": "test"}');

    logs.push({
      timestamp: new Date(now - (count - i) * 1000).toISOString(),
      level: template.level,
      message,
      container: containers[Math.floor(Math.random() * containers.length)],
    });
  }

  return logs;
}

/**
 * Mock data service that returns sample data.
 * Used when DEMO_MODE is enabled.
 *
 * For mock data, workspace name is used as the namespace
 * (workspace name = namespace in demo mode).
 */
export class MockDataService implements DataService {
  readonly name = "MockDataService";
  readonly isDemo = true;

  async getAgents(workspace: string): Promise<AgentRuntime[]> {
    await delay();
    const agents = mockAgentRuntimes as unknown as AgentRuntime[];
    // In demo mode, workspace name = namespace
    return agents.filter((a) => a.metadata?.namespace === workspace);
  }

  async getAgent(workspace: string, name: string): Promise<AgentRuntime | undefined> {
    await delay();
    const agents = mockAgentRuntimes as unknown as AgentRuntime[];
    // In demo mode, workspace name = namespace
    return agents.find(
      (a) => a.metadata?.namespace === workspace && a.metadata?.name === name
    );
  }

  async createAgent(workspace: string, spec: Record<string, unknown>): Promise<AgentRuntime> {
    await delay(500);
    // Return a mock agent in demo mode
    return {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "AgentRuntime",
      metadata: {
        ...(spec.metadata as ObjectMeta),
        namespace: workspace,
      },
      spec: (spec.spec as AgentRuntimeSpec) || {},
      status: {
        phase: "Pending",
      },
    } as AgentRuntime;
  }

  async scaleAgent(
    workspace: string,
    name: string,
    replicas: number
  ): Promise<AgentRuntime> {
    await delay(500);
    // Find the mock agent and return updated version
    // In demo mode, workspace name = namespace
    const agent = mockAgentRuntimes.find(
      (a) => a.metadata.namespace === workspace && a.metadata.name === name
    );
    if (agent) {
      // Return updated copy (mock doesn't persist)
      return {
        ...agent,
        spec: {
          ...agent.spec,
          runtime: { ...agent.spec.runtime, replicas },
        },
        status: {
          ...agent.status,
          replicas: { ...agent.status?.replicas, desired: replicas },
        },
      } as unknown as AgentRuntime;
    }
    throw new Error(`Agent ${workspace}/${name} not found`);
  }

  async getAgentLogs(
    _workspace: string,
    _name: string,
    options?: LogOptions
  ): Promise<LogEntry[]> {
    await delay();
    // Generate realistic mock logs for demo mode
    return generateMockLogs(options?.tailLines || 100);
  }

  async getAgentEvents(workspace: string, name: string): Promise<K8sEvent[]> {
    await delay();
    const now = new Date();
    const minutesAgo = (mins: number) =>
      new Date(now.getTime() - mins * 60 * 1000).toISOString();

    // In demo mode, workspace name = namespace
    const namespace = workspace;

    // Generate realistic mock events for this agent
    const events: K8sEvent[] = [
      {
        type: "Normal",
        reason: "Scheduled",
        message: `Successfully assigned ${namespace}/${name}-xxx to node-1`,
        firstTimestamp: minutesAgo(30),
        lastTimestamp: minutesAgo(30),
        count: 1,
        source: { component: "default-scheduler" },
        involvedObject: { kind: "Pod", name: `${name}-xxx`, namespace },
      },
      {
        type: "Normal",
        reason: "Pulled",
        message: "Container image already present on machine",
        firstTimestamp: minutesAgo(29),
        lastTimestamp: minutesAgo(29),
        count: 1,
        source: { component: "kubelet", host: "node-1" },
        involvedObject: { kind: "Pod", name: `${name}-xxx`, namespace },
      },
      {
        type: "Normal",
        reason: "Created",
        message: "Created container facade",
        firstTimestamp: minutesAgo(29),
        lastTimestamp: minutesAgo(29),
        count: 1,
        source: { component: "kubelet", host: "node-1" },
        involvedObject: { kind: "Pod", name: `${name}-xxx`, namespace },
      },
      {
        type: "Normal",
        reason: "Started",
        message: "Started container facade",
        firstTimestamp: minutesAgo(28),
        lastTimestamp: minutesAgo(28),
        count: 1,
        source: { component: "kubelet", host: "node-1" },
        involvedObject: { kind: "Pod", name: `${name}-xxx`, namespace },
      },
      {
        type: "Normal",
        reason: "Created",
        message: "Created container runtime",
        firstTimestamp: minutesAgo(28),
        lastTimestamp: minutesAgo(28),
        count: 1,
        source: { component: "kubelet", host: "node-1" },
        involvedObject: { kind: "Pod", name: `${name}-xxx`, namespace },
      },
      {
        type: "Normal",
        reason: "Started",
        message: "Started container runtime",
        firstTimestamp: minutesAgo(27),
        lastTimestamp: minutesAgo(27),
        count: 1,
        source: { component: "kubelet", host: "node-1" },
        involvedObject: { kind: "Pod", name: `${name}-xxx`, namespace },
      },
      {
        type: "Normal",
        reason: "PromptPackLoaded",
        message: "Successfully loaded PromptPack version 1.0.0",
        firstTimestamp: minutesAgo(26),
        lastTimestamp: minutesAgo(26),
        count: 1,
        source: { component: "omnia-operator" },
        involvedObject: { kind: "AgentRuntime", name, namespace },
      },
      {
        type: "Normal",
        reason: "Ready",
        message: "Agent runtime is ready to accept connections",
        firstTimestamp: minutesAgo(25),
        lastTimestamp: minutesAgo(25),
        count: 1,
        source: { component: "omnia-operator" },
        involvedObject: { kind: "AgentRuntime", name, namespace },
      },
    ];

    return events.sort(
      (a, b) =>
        new Date(b.lastTimestamp).getTime() - new Date(a.lastTimestamp).getTime()
    );
  }

  async getPromptPacks(workspace: string): Promise<PromptPack[]> {
    await delay();
    const packs = mockPromptPacks as PromptPack[];
    // In demo mode, workspace name = namespace
    return packs.filter((p) => p.metadata?.namespace === workspace);
  }

  async getPromptPack(workspace: string, name: string): Promise<PromptPack | undefined> {
    await delay();
    // In demo mode, workspace name = namespace
    return mockPromptPacks.find(
      (p) => p.metadata?.namespace === workspace && p.metadata?.name === name
    ) as PromptPack | undefined;
  }

  async getPromptPackContent(_workspace: string, _name: string): Promise<PromptPackContent | undefined> {
    await delay();
    // Return mock content
    return {
      id: "mock-prompts",
      name: "Mock Prompts",
      version: "1.0.0",
      description: "Mock prompt pack for demo mode",
      template_engine: {
        version: "v1",
        syntax: "{{variable}}",
      },
      prompts: {
        default: {
          id: "default",
          name: "Default Prompt",
          version: "1.0.0",
          system_template: "You are a helpful AI assistant.",
          parameters: { temperature: 0.7 },
        },
      },
    };
  }

  // Workspace-scoped ToolRegistries (filter by namespace)
  async getToolRegistries(workspace: string): Promise<ToolRegistry[]> {
    await delay();
    // In demo mode, filter by namespace (workspace name = namespace)
    return (mockToolRegistries as ToolRegistry[]).filter(
      (r) => r.metadata?.namespace === workspace
    );
  }

  async getToolRegistry(workspace: string, name: string): Promise<ToolRegistry | undefined> {
    await delay();
    return (mockToolRegistries as ToolRegistry[]).find(
      (r) => r.metadata?.namespace === workspace && r.metadata?.name === name
    );
  }

  // Workspace-scoped Providers (filter by namespace)
  async getProviders(_workspace: string): Promise<Provider[]> {
    await delay();
    // No workspace-scoped mock providers
    return [];
  }

  async getProvider(_workspace: string, _name: string): Promise<Provider | undefined> {
    await delay();
    return undefined;
  }

  // Shared ToolRegistries (system-wide)
  async getSharedToolRegistries(): Promise<ToolRegistry[]> {
    await delay();
    // Return all tool registries as shared (in demo mode)
    return mockToolRegistries as ToolRegistry[];
  }

  async getSharedToolRegistry(name: string): Promise<ToolRegistry | undefined> {
    await delay();
    return mockToolRegistries.find(
      (r) => r.metadata?.name === name
    ) as ToolRegistry | undefined;
  }

  // Shared Providers (system-wide)
  async getSharedProviders(): Promise<Provider[]> {
    await delay();
    // No mock providers
    return [];
  }

  async getSharedProvider(_name: string): Promise<Provider | undefined> {
    await delay();
    return undefined;
  }

  async getStats(_workspace: string): Promise<Stats> {
    await delay();
    return getMockStats() as unknown as Stats;
  }

  async getCosts(_options?: CostOptions): Promise<CostData> {
    await delay();
    return {
      available: true,
      summary: getMockCostSummary(),
      byAgent: mockCostAllocation as CostAllocationItem[],
      byProvider: getMockCostByProvider(),
      byModel: getMockCostByModel(),
      timeSeries: mockCostTimeSeries,
      grafanaUrl: undefined, // No Grafana in demo mode
    };
  }

  // ============================================================
  // Arena Fleet mock methods
  // ============================================================

  async getArenaSources(workspace: string): Promise<ArenaSource[]> {
    await delay();
    return mockArenaSources.filter((s) => s.metadata?.namespace === workspace);
  }

  async getArenaSource(workspace: string, name: string): Promise<ArenaSource | undefined> {
    await delay();
    return mockArenaSources.find(
      (s) => s.metadata?.namespace === workspace && s.metadata?.name === name
    );
  }

  async createArenaSource(workspace: string, name: string, spec: ArenaSourceSpec): Promise<ArenaSource> {
    await delay(500);
    const newSource: ArenaSource = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "ArenaSource",
      metadata: {
        name,
        namespace: workspace,
        uid: `arena-source-${generateId()}`,
        creationTimestamp: new Date().toISOString(),
      },
      spec,
      status: { phase: "Pending" },
    };
    mockArenaSources.push(newSource);
    return newSource;
  }

  async updateArenaSource(workspace: string, name: string, spec: ArenaSourceSpec): Promise<ArenaSource> {
    await delay(500);
    const source = mockArenaSources.find(
      (s) => s.metadata?.namespace === workspace && s.metadata?.name === name
    );
    if (!source) {
      throw new Error(`ArenaSource ${workspace}/${name} not found`);
    }
    source.spec = spec;
    return source;
  }

  async deleteArenaSource(workspace: string, name: string): Promise<void> {
    await delay(500);
    const index = mockArenaSources.findIndex(
      (s) => s.metadata?.namespace === workspace && s.metadata?.name === name
    );
    if (index !== -1) {
      mockArenaSources.splice(index, 1);
    }
  }

  async syncArenaSource(_workspace: string, _name: string): Promise<void> {
    await delay(500);
    // Mock sync does nothing
  }


  async getArenaJobs(workspace: string, options?: ArenaJobListOptions): Promise<ArenaJob[]> {
    await delay();
    let jobs = mockArenaJobs.filter((j) => j.metadata?.namespace === workspace);

    if (options?.type) {
      jobs = jobs.filter((j) => j.spec.type === options.type);
    }
    if (options?.phase) {
      jobs = jobs.filter((j) => j.status?.phase === options.phase);
    }
    if (options?.sourceRef) {
      jobs = jobs.filter((j) => j.spec.sourceRef.name === options.sourceRef);
    }
    if (options?.sort === "recent") {
      jobs.sort((a, b) =>
        new Date(b.metadata?.creationTimestamp || 0).getTime() -
        new Date(a.metadata?.creationTimestamp || 0).getTime()
      );
    } else if (options?.sort === "oldest") {
      jobs.sort((a, b) =>
        new Date(a.metadata?.creationTimestamp || 0).getTime() -
        new Date(b.metadata?.creationTimestamp || 0).getTime()
      );
    }
    if (options?.limit) {
      jobs = jobs.slice(0, options.limit);
    }

    return jobs;
  }

  async getArenaJob(workspace: string, name: string): Promise<ArenaJob | undefined> {
    await delay();
    return mockArenaJobs.find(
      (j) => j.metadata?.namespace === workspace && j.metadata?.name === name
    );
  }

  async getArenaJobResults(_workspace: string, name: string): Promise<ArenaJobResults | undefined> {
    await delay();
    // Return mock evaluation results for completed jobs
    const job = mockArenaJobs.find((j) => j.metadata?.name === name);
    if (!job || job.status?.phase !== "Succeeded") {
      return undefined;
    }
    return {
      jobName: name,
      completedAt: job.status?.completionTime || new Date().toISOString(),
      summary: {
        total: 8,
        passed: 7,
        failed: 1,
        errors: 0,
        skipped: 0,
        passRate: 0.875,
        avgScore: 0.85,
        avgDurationMs: 2500,
      },
      results: [
        { scenario: "greeting", status: "pass", score: 0.95, durationMs: 1200 },
        { scenario: "refund-request", status: "pass", score: 0.88, durationMs: 3200 },
        { scenario: "product-inquiry", status: "pass", score: 0.92, durationMs: 2100 },
        { scenario: "escalation", status: "fail", score: 0.45, durationMs: 4500, error: "Failed to detect urgency" },
      ],
    };
  }

  async getArenaJobMetrics(_workspace: string, name: string): Promise<ArenaJobMetrics | undefined> {
    await delay();
    const job = mockArenaJobs.find((j) => j.metadata?.name === name);
    if (!job || job.status?.phase !== "Running") {
      return undefined;
    }
    return {
      activeWorkers: 2,
      tasksPerSecond: 0.5,
      latencyP50: 2100,
      latencyP95: 4500,
      latencyP99: 6200,
      errorRate: 0.02,
    };
  }

  async createArenaJob(workspace: string, name: string, spec: ArenaJobSpec): Promise<ArenaJob> {
    await delay(500);
    const newJob: ArenaJob = {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "ArenaJob",
      metadata: {
        name,
        namespace: workspace,
        uid: `arena-job-${generateId()}`,
        creationTimestamp: new Date().toISOString(),
      },
      spec,
      status: { phase: "Pending" },
    };
    mockArenaJobs.push(newJob);
    return newJob;
  }

  async cancelArenaJob(workspace: string, name: string): Promise<void> {
    await delay(500);
    const job = mockArenaJobs.find(
      (j) => j.metadata?.namespace === workspace && j.metadata?.name === name
    );
    if (job?.status) {
      job.status.phase = "Cancelled";
    }
  }

  async deleteArenaJob(workspace: string, name: string): Promise<void> {
    await delay(500);
    const index = mockArenaJobs.findIndex(
      (j) => j.metadata?.namespace === workspace && j.metadata?.name === name
    );
    if (index !== -1) {
      mockArenaJobs.splice(index, 1);
    }
  }

  async getArenaJobLogs(
    _workspace: string,
    _name: string,
    options?: LogOptions
  ): Promise<LogEntry[]> {
    await delay();
    // Generate mock logs for arena job workers
    return generateMockLogs(options?.tailLines || 100, ["worker"]);
  }

  async getArenaStats(workspace: string): Promise<ArenaStats> {
    await delay();
    const sources = mockArenaSources.filter((s) => s.metadata?.namespace === workspace);
    const jobs = mockArenaJobs.filter((j) => j.metadata?.namespace === workspace);

    return {
      sources: {
        total: sources.length,
        ready: sources.filter((s) => s.status?.phase === "Ready").length,
        failed: sources.filter((s) => s.status?.phase === "Failed").length,
        active: sources.filter((s) => !s.spec.suspend).length,
      },
      jobs: {
        total: jobs.length,
        running: jobs.filter((j) => j.status?.phase === "Running").length,
        queued: jobs.filter((j) => j.status?.phase === "Pending").length,
        completed: jobs.filter((j) => j.status?.phase === "Succeeded").length,
        failed: jobs.filter((j) => j.status?.phase === "Failed").length,
        successRate: jobs.length > 0
          ? jobs.filter((j) => j.status?.phase === "Succeeded").length / jobs.filter((j) => ["Succeeded", "Failed"].includes(j.status?.phase || "")).length
          : 0,
      },
    };
  }

  createAgentConnection(namespace: string, name: string): AgentConnection {
    // Use real WebSocket connection if WS_PROXY_URL is set (for E2E testing)
    const wsProxyUrl = process.env.NEXT_PUBLIC_WS_PROXY_URL;
    if (wsProxyUrl) {
      return new LiveAgentConnection(namespace, name);
    }
    return new MockAgentConnection(namespace, name);
  }
}

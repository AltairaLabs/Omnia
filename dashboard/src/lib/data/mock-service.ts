/**
 * Mock data service implementation.
 * Returns sample data for demo/testing purposes.
 */

import type {
  DataService,
  AgentRuntime,
  PromptPack,
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
} from "./types";

import type {
  ServerMessage,
  ConnectionStatus,
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
  private messageHandlers: Array<(message: ServerMessage) => void> = [];
  private statusHandlers: Array<(status: ConnectionStatus, error?: string) => void> = [];
  private mockIndex = 0;

  constructor(
    private namespace: string,
    private agentName: string
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

  send(content: string): void {
    if (this.status !== "connected") {
      console.warn("Cannot send message: not connected");
      return;
    }

    // Simulate response
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
    let charIndex = 0;

    // Start streaming chunks
    words.forEach((word, index) => {
      setTimeout(() => {
        this.emitMessage({
          type: "chunk",
          session_id: this.sessionId || undefined,
          content: (index > 0 ? " " : "") + word,
          timestamp: new Date().toISOString(),
        });
        charIndex += word.length + 1;
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

function generateMockLogs(count: number): LogEntry[] {
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
      container: CONTAINERS[Math.floor(Math.random() * CONTAINERS.length)],
    });
  }

  return logs;
}

/**
 * Mock data service that returns sample data.
 * Used when DEMO_MODE is enabled.
 */
export class MockDataService implements DataService {
  readonly name = "MockDataService";
  readonly isDemo = true;

  async getAgents(namespace?: string): Promise<AgentRuntime[]> {
    await delay();
    const agents = mockAgentRuntimes as unknown as AgentRuntime[];
    if (namespace) {
      return agents.filter((a) => a.metadata?.namespace === namespace);
    }
    return agents;
  }

  async getAgent(namespace: string, name: string): Promise<AgentRuntime | undefined> {
    await delay();
    const agents = mockAgentRuntimes as unknown as AgentRuntime[];
    return agents.find(
      (a) => a.metadata?.namespace === namespace && a.metadata?.name === name
    );
  }

  async createAgent(spec: Record<string, unknown>): Promise<AgentRuntime> {
    await delay(500);
    // Return a mock agent in demo mode
    return {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "AgentRuntime",
      metadata: spec.metadata as ObjectMeta,
      spec: (spec.spec as AgentRuntimeSpec) || {},
      status: {
        phase: "Pending",
      },
    } as AgentRuntime;
  }

  async scaleAgent(
    namespace: string,
    name: string,
    replicas: number
  ): Promise<AgentRuntime> {
    await delay(500);
    // Find the mock agent and return updated version
    const agent = mockAgentRuntimes.find(
      (a) => a.metadata.namespace === namespace && a.metadata.name === name
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
    throw new Error(`Agent ${namespace}/${name} not found`);
  }

  async getAgentLogs(
    _namespace: string,
    _name: string,
    options?: LogOptions
  ): Promise<LogEntry[]> {
    await delay();
    // Generate realistic mock logs for demo mode
    return generateMockLogs(options?.tailLines || 100);
  }

  async getAgentEvents(namespace: string, name: string): Promise<K8sEvent[]> {
    await delay();
    const now = new Date();
    const minutesAgo = (mins: number) =>
      new Date(now.getTime() - mins * 60 * 1000).toISOString();

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

  async getPromptPacks(namespace?: string): Promise<PromptPack[]> {
    await delay();
    const packs = mockPromptPacks as PromptPack[];
    if (namespace) {
      return packs.filter((p) => p.metadata?.namespace === namespace);
    }
    return packs;
  }

  async getPromptPack(namespace: string, name: string): Promise<PromptPack | undefined> {
    await delay();
    return mockPromptPacks.find(
      (p) => p.metadata?.namespace === namespace && p.metadata?.name === name
    ) as PromptPack | undefined;
  }

  async getToolRegistries(namespace?: string): Promise<ToolRegistry[]> {
    await delay();
    const registries = mockToolRegistries as ToolRegistry[];
    if (namespace) {
      return registries.filter((r) => r.metadata?.namespace === namespace);
    }
    return registries;
  }

  async getToolRegistry(
    namespace: string,
    name: string
  ): Promise<ToolRegistry | undefined> {
    await delay();
    return mockToolRegistries.find(
      (r) => r.metadata?.namespace === namespace && r.metadata?.name === name
    ) as ToolRegistry | undefined;
  }

  async getProviders(_namespace?: string): Promise<Provider[]> {
    await delay();
    // No mock providers
    return [];
  }

  async getStats(): Promise<Stats> {
    await delay();
    return getMockStats() as unknown as Stats;
  }

  async getNamespaces(): Promise<string[]> {
    await delay();
    return ["default", "production", "staging", "demo"];
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

  createAgentConnection(namespace: string, name: string): AgentConnection {
    return new MockAgentConnection(namespace, name);
  }
}

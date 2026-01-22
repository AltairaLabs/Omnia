/**
 * Live data service that composes multiple backends.
 *
 * This is the main data service used in production (non-demo) mode.
 * It delegates to:
 * - WorkspaceApiService for workspace-scoped CRD data (agents, promptpacks)
 * - Shared API endpoints for system-wide resources (toolregistries, providers)
 * - PrometheusService for cost/metrics data
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
  CostData,
  CostOptions,
  K8sEvent,
  AgentConnection,
  ArenaJobListOptions,
} from "./types";
import type {
  ArenaSource,
  ArenaSourceSpec,
  ArenaConfig,
  ArenaConfigSpec,
  ArenaConfigContent,
  ArenaJob,
  ArenaJobSpec,
  ArenaJobResults,
  ArenaStats,
} from "@/types/arena";
import type {
  ServerMessage,
  ConnectionStatus,
  ClientMessage,
  ContentPart,
} from "@/types/websocket";
import { WorkspaceApiService } from "./workspace-api-service";
import { PrometheusService } from "./prometheus-service";
import { ArenaService, type ArenaJobMetrics } from "./arena-service";
import { getWsProxyUrl } from "@/lib/config";

/**
 * Live agent connection using real WebSocket.
 * Connects through the dashboard's WebSocket proxy to the agent's facade.
 */
export class LiveAgentConnection implements AgentConnection {
  private status: ConnectionStatus = "disconnected";
  private sessionId: string | null = null;
  private maxPayloadSize: number | null = null;
  private ws: WebSocket | null = null;
  private readonly messageHandlers: Array<(message: ServerMessage) => void> = [];
  private readonly statusHandlers: Array<(status: ConnectionStatus, error?: string) => void> = [];

  constructor(
    private readonly namespace: string,
    private readonly agentName: string
  ) {}

  connect(): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      return; // Already connected
    }

    this.setStatus("connecting");

    // Fetch runtime config and then connect
    this.initializeConnection().catch((err) => {
      console.error("Failed to initialize WebSocket connection:", err);
      this.setStatus("error", err instanceof Error ? err.message : "Failed to connect");
    });
  }

  private async initializeConnection(): Promise<void> {
    try {
      const protocol = typeof globalThis !== "undefined" && globalThis.location?.protocol === "https:" ? "wss:" : "ws:";

      // Check build-time env var first, then fall back to runtime config
      let wsProxyUrl = process.env.NEXT_PUBLIC_WS_PROXY_URL;
      if (!wsProxyUrl) {
        // Fetch from runtime config (needed for K8s deployments where config comes from ConfigMap)
        wsProxyUrl = await getWsProxyUrl();
      }
      const wsDirectMode = process.env.NEXT_PUBLIC_WS_DIRECT_MODE === "true";

      let wsUrl: string;
      if (wsProxyUrl && wsDirectMode) {
        // Direct mode: connect directly to the agent's /ws endpoint (for E2E testing)
        // Include agent and namespace as query params (required by facade server)
        wsUrl = `${wsProxyUrl}/ws?agent=${encodeURIComponent(this.agentName)}&namespace=${encodeURIComponent(this.namespace)}`;
      } else if (wsProxyUrl) {
        // Configured proxy URL (dev mode or edge case without ingress)
        wsUrl = `${wsProxyUrl}/api/agents/${this.namespace}/${this.agentName}/ws`;
      } else {
        // Use relative URL - works with gateway/ingress routing in production
        const wsHost = typeof globalThis !== "undefined" && globalThis.location ? globalThis.location.host : "localhost:3002";
        wsUrl = `${protocol}//${wsHost}/api/agents/${this.namespace}/${this.agentName}/ws`;
      }

      this.ws = new WebSocket(wsUrl);

      this.ws.onopen = () => {
        this.setStatus("connected");
      };

      this.ws.onmessage = async (event) => {
        try {
          // Handle both string and Blob data
          let data: string;
          if (event.data instanceof Blob) {
            data = await event.data.text();
          } else {
            data = event.data;
          }

          const message: ServerMessage = JSON.parse(data);

          // Track session ID and capabilities from connected message
          if (message.type === "connected") {
            if (message.session_id) {
              this.sessionId = message.session_id;
            }
            // Extract max payload size from server capabilities
            if (message.connected?.capabilities?.max_payload_size) {
              this.maxPayloadSize = message.connected.capabilities.max_payload_size;
            }
          }

          this.emitMessage(message);
        } catch (err) {
          console.error("Failed to parse WebSocket message:", err);
        }
      };

      this.ws.onerror = () => {
        // WebSocket errors don't expose details for security reasons
        console.warn("[LiveAgentConnection] WebSocket connection failed");
        this.setStatus("error", "WebSocket connection failed");
      };

      this.ws.onclose = (event) => {
        console.warn("[LiveAgentConnection] WebSocket closed:", event.code, event.reason);
        this.ws = null;
        this.sessionId = null;
        this.maxPayloadSize = null;
        // If we got a close code indicating an error, preserve error status
        if (event.code === 1011 || event.code >= 4000) {
          this.setStatus("error", event.reason || "Connection closed unexpectedly");
        } else {
          this.setStatus("disconnected");
        }
      };
    } catch (err) {
      this.setStatus("error", err instanceof Error ? err.message : "Failed to connect");
    }
  }

  disconnect(): void {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.sessionId = null;
    this.maxPayloadSize = null;
    this.setStatus("disconnected");
  }

  send(content: string, options?: { sessionId?: string; parts?: ContentPart[] }): void {
    if (this.ws?.readyState !== WebSocket.OPEN) {
      console.warn("Cannot send message: not connected");
      return;
    }

    const message: ClientMessage = {
      type: "message",
      session_id: options?.sessionId || this.sessionId || undefined,
      content,
      parts: options?.parts,
    };

    this.ws.send(JSON.stringify(message));
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
    return this.maxPayloadSize;
  }

  private setStatus(status: ConnectionStatus, error?: string): void {
    this.status = status;
    this.statusHandlers.forEach((h) => h(status, error));
  }

  private emitMessage(message: ServerMessage): void {
    this.messageHandlers.forEach((h) => h(message));
  }
}

/**
 * Live data service that routes requests to appropriate backends.
 */
export class LiveDataService implements DataService {
  readonly name = "LiveDataService";
  readonly isDemo = false;

  private readonly workspaceService: WorkspaceApiService;
  private readonly prometheusService: PrometheusService;
  private readonly arenaService: ArenaService;

  constructor() {
    this.workspaceService = new WorkspaceApiService();
    this.prometheusService = new PrometheusService();
    this.arenaService = new ArenaService();
  }

  // ============================================================
  // Workspace-scoped data - delegated to WorkspaceApiService
  // ============================================================

  async getAgents(workspace: string): Promise<AgentRuntime[]> {
    return this.workspaceService.getAgents(workspace);
  }

  async getAgent(workspace: string, name: string): Promise<AgentRuntime | undefined> {
    return this.workspaceService.getAgent(workspace, name);
  }

  async createAgent(workspace: string, spec: Record<string, unknown>): Promise<AgentRuntime> {
    return this.workspaceService.createAgent(workspace, spec);
  }

  async scaleAgent(workspace: string, name: string, replicas: number): Promise<AgentRuntime> {
    return this.workspaceService.scaleAgent(workspace, name, replicas);
  }

  async getAgentLogs(workspace: string, name: string, options?: LogOptions): Promise<LogEntry[]> {
    return this.workspaceService.getAgentLogs(workspace, name, options);
  }

  async getAgentEvents(workspace: string, name: string): Promise<K8sEvent[]> {
    return this.workspaceService.getAgentEvents(workspace, name);
  }

  async getPromptPacks(workspace: string): Promise<PromptPack[]> {
    return this.workspaceService.getPromptPacks(workspace);
  }

  async getPromptPack(workspace: string, name: string): Promise<PromptPack | undefined> {
    return this.workspaceService.getPromptPack(workspace, name);
  }

  async getPromptPackContent(workspace: string, name: string): Promise<PromptPackContent | undefined> {
    return this.workspaceService.getPromptPackContent(workspace, name);
  }

  // ============================================================
  // Workspace-scoped ToolRegistries and Providers
  // ============================================================

  async getToolRegistries(workspace: string): Promise<ToolRegistry[]> {
    return this.workspaceService.getToolRegistries(workspace);
  }

  async getToolRegistry(workspace: string, name: string): Promise<ToolRegistry | undefined> {
    return this.workspaceService.getToolRegistry(workspace, name);
  }

  async getProviders(workspace: string): Promise<Provider[]> {
    return this.workspaceService.getProviders(workspace);
  }

  async getProvider(workspace: string, name: string): Promise<Provider | undefined> {
    return this.workspaceService.getProvider(workspace, name);
  }

  // ============================================================
  // Shared/system-wide resources
  // ============================================================

  async getSharedToolRegistries(): Promise<ToolRegistry[]> {
    return this.workspaceService.getSharedToolRegistries();
  }

  async getSharedToolRegistry(name: string): Promise<ToolRegistry | undefined> {
    return this.workspaceService.getSharedToolRegistry(name);
  }

  async getSharedProviders(): Promise<Provider[]> {
    return this.workspaceService.getSharedProviders();
  }

  async getSharedProvider(name: string): Promise<Provider | undefined> {
    return this.workspaceService.getSharedProvider(name);
  }

  async getStats(workspace: string): Promise<Stats> {
    return this.workspaceService.getStats(workspace);
  }

  // ============================================================
  // Cost/metrics data - delegated to PrometheusService
  // ============================================================

  async getCosts(options?: CostOptions): Promise<CostData> {
    return this.prometheusService.getCosts(options);
  }

  // ============================================================
  // Arena Fleet - delegated to ArenaService
  // ============================================================

  async getArenaSources(workspace: string): Promise<ArenaSource[]> {
    return this.arenaService.getArenaSources(workspace);
  }

  async getArenaSource(workspace: string, name: string): Promise<ArenaSource | undefined> {
    return this.arenaService.getArenaSource(workspace, name);
  }

  async createArenaSource(workspace: string, name: string, spec: ArenaSourceSpec): Promise<ArenaSource> {
    return this.arenaService.createArenaSource(workspace, name, spec);
  }

  async updateArenaSource(workspace: string, name: string, spec: ArenaSourceSpec): Promise<ArenaSource> {
    return this.arenaService.updateArenaSource(workspace, name, spec);
  }

  async deleteArenaSource(workspace: string, name: string): Promise<void> {
    return this.arenaService.deleteArenaSource(workspace, name);
  }

  async syncArenaSource(workspace: string, name: string): Promise<void> {
    return this.arenaService.syncArenaSource(workspace, name);
  }

  async getArenaConfigs(workspace: string): Promise<ArenaConfig[]> {
    return this.arenaService.getArenaConfigs(workspace);
  }

  async getArenaConfig(workspace: string, name: string): Promise<ArenaConfig | undefined> {
    return this.arenaService.getArenaConfig(workspace, name);
  }

  async getArenaConfigContent(workspace: string, name: string): Promise<ArenaConfigContent> {
    return this.arenaService.getArenaConfigContent(workspace, name);
  }

  async getArenaConfigFile(workspace: string, configName: string, filePath: string): Promise<string> {
    return this.arenaService.getArenaConfigFile(workspace, configName, filePath);
  }

  async createArenaConfig(workspace: string, name: string, spec: ArenaConfigSpec): Promise<ArenaConfig> {
    return this.arenaService.createArenaConfig(workspace, name, spec);
  }

  async updateArenaConfig(workspace: string, name: string, spec: ArenaConfigSpec): Promise<ArenaConfig> {
    return this.arenaService.updateArenaConfig(workspace, name, spec);
  }

  async deleteArenaConfig(workspace: string, name: string): Promise<void> {
    return this.arenaService.deleteArenaConfig(workspace, name);
  }

  async getArenaJobs(workspace: string, options?: ArenaJobListOptions): Promise<ArenaJob[]> {
    return this.arenaService.getArenaJobs(workspace, options);
  }

  async getArenaJob(workspace: string, name: string): Promise<ArenaJob | undefined> {
    return this.arenaService.getArenaJob(workspace, name);
  }

  async getArenaJobResults(workspace: string, name: string): Promise<ArenaJobResults | undefined> {
    return this.arenaService.getArenaJobResults(workspace, name);
  }

  async getArenaJobMetrics(workspace: string, name: string): Promise<ArenaJobMetrics | undefined> {
    return this.arenaService.getArenaJobMetrics(workspace, name);
  }

  async createArenaJob(workspace: string, name: string, spec: ArenaJobSpec): Promise<ArenaJob> {
    return this.arenaService.createArenaJob(workspace, name, spec);
  }

  async cancelArenaJob(workspace: string, name: string): Promise<void> {
    return this.arenaService.cancelArenaJob(workspace, name);
  }

  async deleteArenaJob(workspace: string, name: string): Promise<void> {
    return this.arenaService.deleteArenaJob(workspace, name);
  }

  async getArenaJobLogs(workspace: string, name: string, options?: LogOptions): Promise<LogEntry[]> {
    return this.arenaService.getArenaJobLogs(workspace, name, options);
  }

  async getArenaStats(workspace: string): Promise<ArenaStats> {
    return this.arenaService.getArenaStats(workspace);
  }

  // ============================================================
  // Agent WebSocket connections
  // ============================================================

  createAgentConnection(namespace: string, name: string): AgentConnection {
    return new LiveAgentConnection(namespace, name);
  }
}

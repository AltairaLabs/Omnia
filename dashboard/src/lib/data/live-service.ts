/**
 * Live data service that composes multiple backends.
 *
 * This is the main data service used in production (non-demo) mode.
 * It delegates to:
 * - OperatorApiService for CRD data (agents, promptpacks, toolregistries)
 * - PrometheusService for cost/metrics data
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
  CostData,
  CostOptions,
  K8sEvent,
  AgentConnection,
} from "./types";
import type {
  ServerMessage,
  ConnectionStatus,
  ClientMessage,
} from "@/types/websocket";
import { OperatorApiService } from "./operator-service";
import { PrometheusService } from "./prometheus-service";

/**
 * Live agent connection using real WebSocket.
 * Connects through the dashboard's WebSocket proxy to the agent's facade.
 */
export class LiveAgentConnection implements AgentConnection {
  private status: ConnectionStatus = "disconnected";
  private sessionId: string | null = null;
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

    try {
      // Connect to the WebSocket proxy server
      // In production, WS_PROXY_URL is configured; in dev, proxy runs on port 3002
      const protocol = typeof globalThis !== "undefined" && globalThis.location?.protocol === "https:" ? "wss:" : "ws:";
      const wsProxyUrl = process.env.NEXT_PUBLIC_WS_PROXY_URL;

      let wsUrl: string;
      if (wsProxyUrl) {
        // Production: use configured proxy URL
        wsUrl = `${wsProxyUrl}/api/agents/${this.namespace}/${this.agentName}/ws`;
      } else {
        // Development: WebSocket proxy on port 3002
        const wsHost = typeof globalThis !== "undefined" && globalThis.location ? globalThis.location.hostname : "localhost";
        wsUrl = `${protocol}//${wsHost}:3002/api/agents/${this.namespace}/${this.agentName}/ws`;
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

          // Track session ID from connected message
          if (message.type === "connected" && message.session_id) {
            this.sessionId = message.session_id;
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
    this.setStatus("disconnected");
  }

  send(content: string, sessionId?: string): void {
    if (this.ws?.readyState !== WebSocket.OPEN) {
      console.warn("Cannot send message: not connected");
      return;
    }

    const message: ClientMessage = {
      type: "message",
      session_id: sessionId || this.sessionId || undefined,
      content,
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

  private readonly operatorService: OperatorApiService;
  private readonly prometheusService: PrometheusService;

  constructor() {
    this.operatorService = new OperatorApiService();
    this.prometheusService = new PrometheusService();
  }

  // ============================================================
  // CRD data - delegated to OperatorApiService
  // ============================================================

  async getAgents(namespace?: string): Promise<AgentRuntime[]> {
    return this.operatorService.getAgents(namespace);
  }

  async getAgent(namespace: string, name: string): Promise<AgentRuntime | undefined> {
    return this.operatorService.getAgent(namespace, name);
  }

  async createAgent(spec: Record<string, unknown>): Promise<AgentRuntime> {
    return this.operatorService.createAgent(spec);
  }

  async scaleAgent(namespace: string, name: string, replicas: number): Promise<AgentRuntime> {
    return this.operatorService.scaleAgent(namespace, name, replicas);
  }

  async getAgentLogs(namespace: string, name: string, options?: LogOptions): Promise<LogEntry[]> {
    return this.operatorService.getAgentLogs(namespace, name, options);
  }

  async getAgentEvents(namespace: string, name: string): Promise<K8sEvent[]> {
    return this.operatorService.getAgentEvents(namespace, name);
  }

  async getPromptPacks(namespace?: string): Promise<PromptPack[]> {
    return this.operatorService.getPromptPacks(namespace);
  }

  async getPromptPack(namespace: string, name: string): Promise<PromptPack | undefined> {
    return this.operatorService.getPromptPack(namespace, name);
  }

  async getToolRegistries(namespace?: string): Promise<ToolRegistry[]> {
    return this.operatorService.getToolRegistries(namespace);
  }

  async getToolRegistry(namespace: string, name: string): Promise<ToolRegistry | undefined> {
    return this.operatorService.getToolRegistry(namespace, name);
  }

  async getProviders(namespace?: string): Promise<Provider[]> {
    return this.operatorService.getProviders(namespace);
  }

  async getStats(): Promise<Stats> {
    return this.operatorService.getStats();
  }

  async getNamespaces(): Promise<string[]> {
    return this.operatorService.getNamespaces();
  }

  // ============================================================
  // Cost/metrics data - delegated to PrometheusService
  // ============================================================

  async getCosts(options?: CostOptions): Promise<CostData> {
    return this.prometheusService.getCosts(options);
  }

  // ============================================================
  // Agent WebSocket connections
  // ============================================================

  createAgentConnection(namespace: string, name: string): AgentConnection {
    return new LiveAgentConnection(namespace, name);
  }
}

/**
 * Live data service that composes multiple backends.
 *
 * This is the main data service used in production (non-demo) mode.
 * It delegates to:
 * - WorkspaceApiService for workspace-scoped CRD data (agents, promptpacks)
 * - Shared API endpoints for system-wide resources (toolregistries, providers)
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
  K8sEvent,
  AgentConnection,
  ArenaJobListOptions,
  SessionListOptions,
  SessionSearchOptions,
  SessionMessageOptions,
  SessionListResponse,
  SessionMessagesResponse,
  MemoryEntity,
  MemoryListResponse,
  MemoryListOptions,
  MemorySearchOptions,
} from "./types";
import type { Session } from "@/types/session";
import type {
  ArenaSource,
  ArenaSourceSpec,
  ArenaJob,
  ArenaJobSpec,
  ArenaJobResults,
  ArenaStats,
} from "@/types/arena";
import { WorkspaceApiService } from "./workspace-api-service";
import { ArenaService, type ArenaJobMetrics } from "./arena-service";
import { SessionApiService } from "./session-api-service";
import { MemoryApiService } from "./memory-api-service";
import { LiveAgentConnection } from "./live-agent-connection";

// Re-export so existing importers (index.ts, tests, consumers) keep working unchanged.
export { LiveAgentConnection } from "./live-agent-connection";
export type { ConnectedEventInfo } from "./live-agent-connection";

/**
 * Live data service that routes requests to appropriate backends.
 */
export class LiveDataService implements DataService {
  readonly name = "LiveDataService";
  readonly isDemo = false;

  private readonly workspaceService: WorkspaceApiService;
  private readonly arenaService: ArenaService;
  private readonly sessionService: SessionApiService;
  private readonly memoryService: MemoryApiService;

  constructor() {
    this.workspaceService = new WorkspaceApiService();
    this.arenaService = new ArenaService();
    this.sessionService = new SessionApiService();
    this.memoryService = new MemoryApiService();
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

  async updateAgentEvals(
    workspace: string,
    name: string,
    evals: import("./types").AgentEvalsPatch,
  ): Promise<AgentRuntime> {
    return this.workspaceService.updateAgentEvals(workspace, name, evals);
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
  // Sessions - delegated to SessionApiService
  // ============================================================

  async getSessions(workspace: string, options?: SessionListOptions): Promise<SessionListResponse> {
    return this.sessionService.getSessions(workspace, options);
  }

  async getSessionById(workspace: string, sessionId: string): Promise<Session | undefined> {
    return this.sessionService.getSessionById(workspace, sessionId);
  }

  async searchSessions(workspace: string, options: SessionSearchOptions): Promise<SessionListResponse> {
    return this.sessionService.searchSessions(workspace, options);
  }

  async getSessionMessages(workspace: string, sessionId: string, options?: SessionMessageOptions): Promise<SessionMessagesResponse> {
    return this.sessionService.getSessionMessages(workspace, sessionId, options);
  }

  // ============================================================
  // Memory - delegated to MemoryApiService
  // ============================================================

  async getMemories(options: MemoryListOptions): Promise<MemoryListResponse> {
    return this.memoryService.getMemories(options);
  }

  async searchMemories(options: MemorySearchOptions): Promise<MemoryListResponse> {
    return this.memoryService.searchMemories(options);
  }

  async exportMemories(workspace: string, userId: string): Promise<MemoryEntity[]> {
    return this.memoryService.exportMemories(workspace, userId);
  }

  async deleteMemory(workspace: string, memoryId: string): Promise<void> {
    return this.memoryService.deleteMemory(workspace, memoryId);
  }

  async deleteAllMemories(workspace: string, userId: string): Promise<void> {
    return this.memoryService.deleteAllMemories(workspace, userId);
  }

  // ============================================================
  // Agent WebSocket connections
  // ============================================================

  createAgentConnection(namespace: string, name: string): AgentConnection {
    return new LiveAgentConnection(namespace, name);
  }
}

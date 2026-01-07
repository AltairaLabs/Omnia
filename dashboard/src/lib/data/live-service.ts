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
} from "./types";
import { OperatorApiService } from "./operator-service";
import { PrometheusService } from "./prometheus-service";

/**
 * Live data service that routes requests to appropriate backends.
 */
export class LiveDataService implements DataService {
  readonly name = "LiveDataService";
  readonly isDemo = false;

  private operatorService: OperatorApiService;
  private prometheusService: PrometheusService;

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
}

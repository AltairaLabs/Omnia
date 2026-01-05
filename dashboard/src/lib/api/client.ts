/**
 * Auto-generated API client using openapi-fetch.
 * Types are generated from the OpenAPI spec at api/openapi/openapi.yaml.
 *
 * Run `npm run generate:api` to regenerate types after changing the spec.
 */
import createClient from "openapi-fetch";
import type { paths, components } from "./schema";

// Environment configuration
const OPERATOR_API_URL =
  process.env.NEXT_PUBLIC_OPERATOR_API_URL || "http://localhost:8082";
export const isDemoMode = process.env.NEXT_PUBLIC_DEMO_MODE === "true";

// Create typed API client
const client = createClient<paths>({ baseUrl: OPERATOR_API_URL });

// Re-export component types for convenience
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

// Mock data for demo mode
import {
  mockAgentRuntimes,
  mockPromptPacks,
  mockToolRegistries,
  getMockStats,
} from "../mock-data";

/**
 * Fetch all agents, optionally filtered by namespace.
 */
export async function fetchAgents(namespace?: string): Promise<AgentRuntime[]> {
  if (isDemoMode) {
    await simulateDelay();
    return mockAgentRuntimes as unknown as AgentRuntime[];
  }

  const { data, error } = await client.GET("/api/v1/agents", {
    params: { query: namespace ? { namespace } : {} },
  });

  if (error) {
    throw new Error(`Failed to fetch agents: ${JSON.stringify(error)}`);
  }

  return data ?? [];
}

/**
 * Fetch a specific agent by namespace and name.
 */
export async function fetchAgent(
  namespace: string,
  name: string
): Promise<AgentRuntime | undefined> {
  if (isDemoMode) {
    await simulateDelay();
    const agent = mockAgentRuntimes.find(
      (a) => a.metadata.namespace === namespace && a.metadata.name === name
    );
    return agent as unknown as AgentRuntime | undefined;
  }

  const { data, error } = await client.GET("/api/v1/agents/{namespace}/{name}", {
    params: { path: { namespace, name } },
  });

  if (error) {
    // Return undefined for not found
    if (JSON.stringify(error).includes("not found")) {
      return undefined;
    }
    throw new Error(`Failed to fetch agent: ${JSON.stringify(error)}`);
  }

  return data;
}

/**
 * Fetch all prompt packs, optionally filtered by namespace.
 */
export async function fetchPromptPacks(namespace?: string): Promise<PromptPack[]> {
  if (isDemoMode) {
    await simulateDelay();
    return mockPromptPacks as PromptPack[];
  }

  const { data, error } = await client.GET("/api/v1/promptpacks", {
    params: { query: namespace ? { namespace } : {} },
  });

  if (error) {
    throw new Error(`Failed to fetch prompt packs: ${JSON.stringify(error)}`);
  }

  return data ?? [];
}

/**
 * Fetch a specific prompt pack by namespace and name.
 */
export async function fetchPromptPack(
  namespace: string,
  name: string
): Promise<PromptPack | undefined> {
  if (isDemoMode) {
    await simulateDelay();
    return mockPromptPacks.find(
      (p) => p.metadata?.namespace === namespace && p.metadata?.name === name
    ) as PromptPack | undefined;
  }

  const { data, error } = await client.GET("/api/v1/promptpacks/{namespace}/{name}", {
    params: { path: { namespace, name } },
  });

  if (error) {
    if (JSON.stringify(error).includes("not found")) {
      return undefined;
    }
    throw new Error(`Failed to fetch prompt pack: ${JSON.stringify(error)}`);
  }

  return data;
}

/**
 * Fetch all tool registries, optionally filtered by namespace.
 */
export async function fetchToolRegistries(namespace?: string): Promise<ToolRegistry[]> {
  if (isDemoMode) {
    await simulateDelay();
    return mockToolRegistries as ToolRegistry[];
  }

  const { data, error } = await client.GET("/api/v1/toolregistries", {
    params: { query: namespace ? { namespace } : {} },
  });

  if (error) {
    throw new Error(`Failed to fetch tool registries: ${JSON.stringify(error)}`);
  }

  return data ?? [];
}

/**
 * Fetch a specific tool registry by namespace and name.
 */
export async function fetchToolRegistry(
  namespace: string,
  name: string
): Promise<ToolRegistry | undefined> {
  if (isDemoMode) {
    await simulateDelay();
    return mockToolRegistries.find(
      (r) => r.metadata?.namespace === namespace && r.metadata?.name === name
    ) as ToolRegistry | undefined;
  }

  const { data, error } = await client.GET("/api/v1/toolregistries/{namespace}/{name}", {
    params: { path: { namespace, name } },
  });

  if (error) {
    if (JSON.stringify(error).includes("not found")) {
      return undefined;
    }
    throw new Error(`Failed to fetch tool registry: ${JSON.stringify(error)}`);
  }

  return data;
}

/**
 * Fetch all providers, optionally filtered by namespace.
 */
export async function fetchProviders(namespace?: string): Promise<Provider[]> {
  if (isDemoMode) {
    await simulateDelay();
    return []; // No mock providers
  }

  const { data, error } = await client.GET("/api/v1/providers", {
    params: { query: namespace ? { namespace } : {} },
  });

  if (error) {
    throw new Error(`Failed to fetch providers: ${JSON.stringify(error)}`);
  }

  return data ?? [];
}

/**
 * Fetch aggregated statistics.
 */
export async function fetchStats(): Promise<Stats> {
  if (isDemoMode) {
    await simulateDelay();
    return getMockStats() as unknown as Stats;
  }

  const { data, error } = await client.GET("/api/v1/stats");

  if (error) {
    throw new Error(`Failed to fetch stats: ${JSON.stringify(error)}`);
  }

  return data ?? {
    agents: { total: 0, running: 0, pending: 0, failed: 0 },
    promptPacks: { total: 0, active: 0, canary: 0 },
    tools: { total: 0, available: 0, degraded: 0 },
  };
}

/**
 * Fetch logs for an agent.
 */
export async function fetchAgentLogs(
  namespace: string,
  name: string,
  options?: {
    tailLines?: number;
    sinceSeconds?: number;
    container?: string;
  }
): Promise<LogEntry[]> {
  if (isDemoMode) {
    await simulateDelay();
    return []; // No mock logs - the LogViewer will generate mock data
  }

  const { data, error } = await client.GET("/api/v1/agents/{namespace}/{name}/logs", {
    params: {
      path: { namespace, name },
      query: {
        tailLines: options?.tailLines,
        sinceSeconds: options?.sinceSeconds,
        container: options?.container,
      },
    },
  });

  if (error) {
    throw new Error(`Failed to fetch logs: ${JSON.stringify(error)}`);
  }

  return data ?? [];
}

// Helper to simulate network delay in demo mode
function simulateDelay(ms = 100): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

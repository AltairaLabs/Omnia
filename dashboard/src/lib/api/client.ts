/**
 * Auto-generated API client using openapi-fetch.
 * Types are generated from the OpenAPI spec at api/openapi/openapi.yaml.
 *
 * Run `npm run generate:api` to regenerate types after changing the spec.
 *
 * All requests go through the server-side proxy at /api/operator which:
 * - Forwards requests to the operator API in production
 * - Returns mock data when NEXT_PUBLIC_DEMO_MODE=true (read at runtime)
 */
import createClient from "openapi-fetch";
import type { paths, components } from "./schema";

// Use the server-side proxy for API calls.
// This allows the dashboard to work when deployed in-cluster without
// exposing the operator API externally. The proxy at /api/operator
// forwards requests to the actual operator service.
// In demo mode, the proxy returns mock data instead.
const API_BASE_URL = "/api/operator";

// Create typed API client using the proxy
const client = createClient<paths>({ baseUrl: API_BASE_URL });

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

/**
 * Fetch all agents, optionally filtered by namespace.
 */
export async function fetchAgents(namespace?: string): Promise<AgentRuntime[]> {
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
 * Fetch all namespaces.
 */
export async function fetchNamespaces(): Promise<string[]> {
  const { data, error } = await client.GET("/api/v1/namespaces");

  if (error) {
    throw new Error(`Failed to fetch namespaces: ${JSON.stringify(error)}`);
  }

  return data ?? [];
}

/**
 * Create a new agent.
 */
export async function createAgent(spec: Record<string, unknown>): Promise<AgentRuntime> {
  const response = await fetch(`${API_BASE_URL}/v1/agents`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(spec),
  });

  if (!response.ok) {
    const errorText = await response.text();
    throw new Error(`Failed to create agent: ${errorText}`);
  }

  return response.json();
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

/**
 * Scale an agent to a specific number of replicas.
 */
export async function scaleAgent(
  namespace: string,
  name: string,
  replicas: number
): Promise<AgentRuntime> {
  const { data, error } = await client.PUT("/api/v1/agents/{namespace}/{name}/scale", {
    params: { path: { namespace, name } },
    body: { replicas },
  });

  if (error) {
    throw new Error(`Failed to scale agent: ${JSON.stringify(error)}`);
  }

  return data as AgentRuntime;
}

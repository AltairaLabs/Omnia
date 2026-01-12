/**
 * Operator API data service implementation.
 * Calls the real operator API through the Next.js proxy.
 */

import createClient from "openapi-fetch";
import type { paths } from "../api/schema";
import type {
  AgentRuntime,
  PromptPack,
  PromptPackContent,
  ToolRegistry,
  Provider,
  Stats,
  LogEntry,
  LogOptions,
  K8sEvent,
} from "./types";

// Proxy base URL - requests go through Next.js API routes
const API_BASE_URL = "/api/operator";

/**
 * Operator API service that calls the real Kubernetes operator.
 * This is a lower-level service used by LiveDataService - it does not implement
 * the full DataService interface since cost data comes from PrometheusService.
 */
export class OperatorApiService {
  readonly name = "OperatorApiService";

  private readonly client = createClient<paths>({ baseUrl: API_BASE_URL });

  async getAgents(namespace?: string): Promise<AgentRuntime[]> {
    const { data, error } = await this.client.GET("/api/v1/agents", {
      params: { query: namespace ? { namespace } : {} },
    });
    if (error) {
      throw new Error(`Failed to fetch agents: ${JSON.stringify(error)}`);
    }
    return (data ?? []) as AgentRuntime[];
  }

  async getAgent(namespace: string, name: string): Promise<AgentRuntime | undefined> {
    const { data, error } = await this.client.GET("/api/v1/agents/{namespace}/{name}", {
      params: { path: { namespace, name } },
    });
    if (error) {
      // 404 means not found, return undefined
      if (typeof error === "object" && "code" in error && error.code === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch agent: ${JSON.stringify(error)}`);
    }
    return data as AgentRuntime | undefined;
  }

  async createAgent(spec: Record<string, unknown>): Promise<AgentRuntime> {
    // Use fetch directly for POST since openapi-fetch typing is complex for this endpoint
    const response = await fetch(`${API_BASE_URL}/api/v1/agents`, {
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

    return response.json() as Promise<AgentRuntime>;
  }

  async scaleAgent(
    namespace: string,
    name: string,
    replicas: number
  ): Promise<AgentRuntime> {
    // Use fetch directly for PUT scale endpoint
    const response = await fetch(
      `${API_BASE_URL}/api/v1/agents/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/scale`,
      {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ replicas }),
      }
    );

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Failed to scale agent: ${errorText}`);
    }

    return response.json() as Promise<AgentRuntime>;
  }

  async getAgentLogs(
    namespace: string,
    name: string,
    options?: LogOptions
  ): Promise<LogEntry[]> {
    const { data, error } = await this.client.GET(
      "/api/v1/agents/{namespace}/{name}/logs",
      {
        params: {
          path: { namespace, name },
          query: {
            tailLines: options?.tailLines,
            sinceSeconds: options?.sinceSeconds,
            container: options?.container,
          },
        },
      }
    );
    if (error) {
      throw new Error(`Failed to fetch logs: ${JSON.stringify(error)}`);
    }
    return (data ?? []) as LogEntry[];
  }

  async getAgentEvents(namespace: string, name: string): Promise<K8sEvent[]> {
    // Call the events endpoint - returns K8s events related to this agent
    const response = await fetch(
      `${API_BASE_URL}/api/v1/agents/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/events`
    );

    if (!response.ok) {
      // If endpoint doesn't exist yet (404) or other error, return empty array
      if (response.status === 404) {
        console.warn("Events endpoint not available");
        return [];
      }
      throw new Error(`Failed to fetch events: ${response.statusText}`);
    }

    return response.json() as Promise<K8sEvent[]>;
  }

  async getPromptPacks(namespace?: string): Promise<PromptPack[]> {
    const { data, error } = await this.client.GET("/api/v1/promptpacks", {
      params: { query: namespace ? { namespace } : {} },
    });
    if (error) {
      throw new Error(`Failed to fetch prompt packs: ${JSON.stringify(error)}`);
    }
    return (data ?? []) as PromptPack[];
  }

  async getPromptPack(
    namespace: string,
    name: string
  ): Promise<PromptPack | undefined> {
    const { data, error } = await this.client.GET(
      "/api/v1/promptpacks/{namespace}/{name}",
      {
        params: { path: { namespace, name } },
      }
    );
    if (error) {
      if (typeof error === "object" && "code" in error && error.code === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch prompt pack: ${JSON.stringify(error)}`);
    }
    return data as PromptPack | undefined;
  }

  async getPromptPackContent(
    namespace: string,
    name: string
  ): Promise<PromptPackContent | undefined> {
    // Call the content endpoint directly (not in OpenAPI schema yet)
    const response = await fetch(
      `${API_BASE_URL}/api/v1/promptpacks/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/content`
    );
    if (!response.ok) {
      if (response.status === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch prompt pack content: ${response.statusText}`);
    }
    return response.json() as Promise<PromptPackContent>;
  }

  async getToolRegistries(namespace?: string): Promise<ToolRegistry[]> {
    const { data, error } = await this.client.GET("/api/v1/toolregistries", {
      params: { query: namespace ? { namespace } : {} },
    });
    if (error) {
      throw new Error(`Failed to fetch tool registries: ${JSON.stringify(error)}`);
    }
    return (data ?? []) as ToolRegistry[];
  }

  async getToolRegistry(
    namespace: string,
    name: string
  ): Promise<ToolRegistry | undefined> {
    const { data, error } = await this.client.GET(
      "/api/v1/toolregistries/{namespace}/{name}",
      {
        params: { path: { namespace, name } },
      }
    );
    if (error) {
      if (typeof error === "object" && "code" in error && error.code === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch tool registry: ${JSON.stringify(error)}`);
    }
    return data as ToolRegistry | undefined;
  }

  async getProviders(namespace?: string): Promise<Provider[]> {
    const { data, error } = await this.client.GET("/api/v1/providers", {
      params: { query: namespace ? { namespace } : {} },
    });
    if (error) {
      throw new Error(`Failed to fetch providers: ${JSON.stringify(error)}`);
    }
    return (data ?? []) as Provider[];
  }

  async getStats(): Promise<Stats> {
    const { data, error } = await this.client.GET("/api/v1/stats");
    if (error) {
      throw new Error(`Failed to fetch stats: ${JSON.stringify(error)}`);
    }
    return data as Stats;
  }

  async getNamespaces(): Promise<string[]> {
    const { data, error } = await this.client.GET("/api/v1/namespaces");
    if (error) {
      throw new Error(`Failed to fetch namespaces: ${JSON.stringify(error)}`);
    }
    return (data ?? []) as string[];
  }
}

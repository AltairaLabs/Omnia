/**
 * Workspace API service for workspace-scoped data fetching.
 *
 * Calls the workspace-scoped API routes:
 *   /api/workspaces/{name}/agents
 *   /api/workspaces/{name}/promptpacks
 *   etc.
 *
 * This service is used by LiveDataService when fetching workspace resources.
 */

import type {
  AgentRuntime,
  PromptPack,
  PromptPackContent,
  ToolRegistry,
  Provider,
  LogEntry,
  LogOptions,
  K8sEvent,
  Stats,
} from "./types";

/**
 * Workspace API service that calls workspace-scoped endpoints.
 */
export class WorkspaceApiService {
  readonly name = "WorkspaceApiService";

  // ============================================================
  // Agents
  // ============================================================

  async getAgents(workspace: string): Promise<AgentRuntime[]> {
    const response = await fetch(`/api/workspaces/${encodeURIComponent(workspace)}/agents`);
    if (!response.ok) {
      if (response.status === 401 || response.status === 403) {
        return []; // No access
      }
      if (response.status === 404) {
        return []; // Workspace not found
      }
      throw new Error(`Failed to fetch agents: ${response.statusText}`);
    }
    return response.json();
  }

  async getAgent(workspace: string, name: string): Promise<AgentRuntime | undefined> {
    const response = await fetch(
      `/api/workspaces/${encodeURIComponent(workspace)}/agents/${encodeURIComponent(name)}`
    );
    if (!response.ok) {
      if (response.status === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch agent: ${response.statusText}`);
    }
    return response.json();
  }

  async createAgent(workspace: string, spec: Record<string, unknown>): Promise<AgentRuntime> {
    const response = await fetch(`/api/workspaces/${encodeURIComponent(workspace)}/agents`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(spec),
    });
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Failed to create agent: ${errorText}`);
    }
    return response.json();
  }

  async scaleAgent(workspace: string, name: string, replicas: number): Promise<AgentRuntime> {
    const response = await fetch(
      `/api/workspaces/${encodeURIComponent(workspace)}/agents/${encodeURIComponent(name)}/scale`,
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ replicas }),
      }
    );
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Failed to scale agent: ${errorText}`);
    }
    return response.json();
  }

  async getAgentLogs(
    workspace: string,
    name: string,
    options?: LogOptions
  ): Promise<LogEntry[]> {
    const params = new URLSearchParams();
    if (options?.tailLines) params.set("tailLines", String(options.tailLines));
    if (options?.sinceSeconds) params.set("sinceSeconds", String(options.sinceSeconds));
    if (options?.container) params.set("container", options.container);

    const queryString = params.toString();
    const suffix = queryString ? "?" + queryString : "";
    const url = `/api/workspaces/${encodeURIComponent(workspace)}/agents/${encodeURIComponent(name)}/logs${suffix}`;

    const response = await fetch(url);
    if (!response.ok) {
      throw new Error(`Failed to fetch logs: ${response.statusText}`);
    }
    return response.json();
  }

  async getAgentEvents(workspace: string, name: string): Promise<K8sEvent[]> {
    const response = await fetch(
      `/api/workspaces/${encodeURIComponent(workspace)}/agents/${encodeURIComponent(name)}/events`
    );
    if (!response.ok) {
      if (response.status === 404) {
        return [];
      }
      throw new Error(`Failed to fetch events: ${response.statusText}`);
    }
    return response.json();
  }

  // ============================================================
  // PromptPacks
  // ============================================================

  async getPromptPacks(workspace: string): Promise<PromptPack[]> {
    const response = await fetch(`/api/workspaces/${encodeURIComponent(workspace)}/promptpacks`);
    if (!response.ok) {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        return [];
      }
      throw new Error(`Failed to fetch prompt packs: ${response.statusText}`);
    }
    return response.json();
  }

  async getPromptPack(workspace: string, name: string): Promise<PromptPack | undefined> {
    const response = await fetch(
      `/api/workspaces/${encodeURIComponent(workspace)}/promptpacks/${encodeURIComponent(name)}`
    );
    if (!response.ok) {
      if (response.status === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch prompt pack: ${response.statusText}`);
    }
    return response.json();
  }

  async getPromptPackContent(workspace: string, name: string): Promise<PromptPackContent | undefined> {
    const response = await fetch(
      `/api/workspaces/${encodeURIComponent(workspace)}/promptpacks/${encodeURIComponent(name)}/content`
    );
    if (!response.ok) {
      if (response.status === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch prompt pack content: ${response.statusText}`);
    }
    return response.json();
  }

  // ============================================================
  // Workspace-scoped ToolRegistries
  // ============================================================

  async getToolRegistries(workspace: string): Promise<ToolRegistry[]> {
    const response = await fetch(`/api/workspaces/${encodeURIComponent(workspace)}/toolregistries`);
    if (!response.ok) {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        return [];
      }
      throw new Error(`Failed to fetch tool registries: ${response.statusText}`);
    }
    return response.json();
  }

  async getToolRegistry(workspace: string, name: string): Promise<ToolRegistry | undefined> {
    const response = await fetch(
      `/api/workspaces/${encodeURIComponent(workspace)}/toolregistries/${encodeURIComponent(name)}`
    );
    if (!response.ok) {
      if (response.status === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch tool registry: ${response.statusText}`);
    }
    return response.json();
  }

  // ============================================================
  // Workspace-scoped Providers
  // ============================================================

  async getProviders(workspace: string): Promise<Provider[]> {
    const response = await fetch(`/api/workspaces/${encodeURIComponent(workspace)}/providers`);
    if (!response.ok) {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        return [];
      }
      throw new Error(`Failed to fetch providers: ${response.statusText}`);
    }
    return response.json();
  }

  async getProvider(workspace: string, name: string): Promise<Provider | undefined> {
    const response = await fetch(
      `/api/workspaces/${encodeURIComponent(workspace)}/providers/${encodeURIComponent(name)}`
    );
    if (!response.ok) {
      if (response.status === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch provider: ${response.statusText}`);
    }
    return response.json();
  }

  // ============================================================
  // Shared resources (read-only, system-wide)
  // ============================================================

  async getSharedToolRegistries(): Promise<ToolRegistry[]> {
    const response = await fetch("/api/shared/toolregistries");
    if (!response.ok) {
      if (response.status === 401 || response.status === 403) {
        return [];
      }
      throw new Error(`Failed to fetch shared tool registries: ${response.statusText}`);
    }
    return response.json();
  }

  async getSharedToolRegistry(name: string): Promise<ToolRegistry | undefined> {
    const response = await fetch(`/api/shared/toolregistries/${encodeURIComponent(name)}`);
    if (!response.ok) {
      if (response.status === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch shared tool registry: ${response.statusText}`);
    }
    return response.json();
  }

  async getSharedProviders(): Promise<Provider[]> {
    const response = await fetch("/api/shared/providers");
    if (!response.ok) {
      if (response.status === 401 || response.status === 403) {
        return [];
      }
      throw new Error(`Failed to fetch shared providers: ${response.statusText}`);
    }
    return response.json();
  }

  async getSharedProvider(name: string): Promise<Provider | undefined> {
    const response = await fetch(`/api/shared/providers/${encodeURIComponent(name)}`);
    if (!response.ok) {
      if (response.status === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch shared provider: ${response.statusText}`);
    }
    return response.json();
  }

  // ============================================================
  // Stats
  // ============================================================

  async getStats(workspace: string): Promise<Stats> {
    const response = await fetch(`/api/workspaces/${encodeURIComponent(workspace)}/stats`);
    if (!response.ok) {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        // Return empty stats if no access
        return {
          agents: { total: 0, running: 0, pending: 0, failed: 0 },
          promptPacks: { total: 0, active: 0, canary: 0, pending: 0, failed: 0 },
          tools: { total: 0, available: 0, degraded: 0, unavailable: 0 },
        };
      }
      throw new Error(`Failed to fetch stats: ${response.statusText}`);
    }
    return response.json();
  }
}

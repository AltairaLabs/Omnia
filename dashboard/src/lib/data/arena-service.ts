/**
 * Arena API service for Arena Fleet resources.
 *
 * Calls the workspace-scoped Arena API routes:
 *   /api/workspaces/{name}/arena/sources
 *   /api/workspaces/{name}/arena/configs
 *   /api/workspaces/{name}/arena/jobs
 *   /api/workspaces/{name}/arena/stats
 *
 * This service is used by LiveDataService when fetching Arena resources.
 */

import type {
  ArenaSource,
  ArenaSourceSpec,
  ArenaConfig,
  ArenaConfigSpec,
  ArenaJob,
  ArenaJobSpec,
  ArenaJobResults,
  ArenaJobMetrics,
  ArenaStats,
  Scenario,
} from "@/types/arena";

const ARENA_API_BASE = "/api/workspaces";

/** Options for listing Arena jobs */
export interface ArenaJobListOptions {
  /** Filter by job type */
  type?: "evaluation" | "loadtest" | "datagen";
  /** Filter by phase */
  phase?: "Pending" | "Running" | "Completed" | "Failed" | "Cancelled";
  /** Filter by config reference */
  configRef?: string;
  /** Maximum number of results */
  limit?: number;
  /** Sort order */
  sort?: "recent" | "oldest" | "name";
}

// Re-export ArenaJobMetrics for consumers that import from this file
export type { ArenaJobMetrics };

/**
 * Arena API service that calls workspace-scoped Arena endpoints.
 */
export class ArenaService {
  readonly name = "ArenaService";

  // ============================================================
  // ArenaSources
  // ============================================================

  async getArenaSources(workspace: string): Promise<ArenaSource[]> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/sources`
    );
    if (!response.ok) {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        return [];
      }
      throw new Error(`Failed to fetch arena sources: ${response.statusText}`);
    }
    return response.json();
  }

  async getArenaSource(workspace: string, name: string): Promise<ArenaSource | undefined> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/sources/${encodeURIComponent(name)}`
    );
    if (!response.ok) {
      if (response.status === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch arena source: ${response.statusText}`);
    }
    return response.json();
  }

  async createArenaSource(
    workspace: string,
    name: string,
    spec: ArenaSourceSpec
  ): Promise<ArenaSource> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/sources`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ metadata: { name }, spec }),
      }
    );
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || "Failed to create arena source");
    }
    return response.json();
  }

  async updateArenaSource(
    workspace: string,
    name: string,
    spec: ArenaSourceSpec
  ): Promise<ArenaSource> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/sources/${encodeURIComponent(name)}`,
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ spec }),
      }
    );
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || "Failed to update arena source");
    }
    return response.json();
  }

  async deleteArenaSource(workspace: string, name: string): Promise<void> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/sources/${encodeURIComponent(name)}`,
      { method: "DELETE" }
    );
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || "Failed to delete arena source");
    }
  }

  async syncArenaSource(workspace: string, name: string): Promise<void> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/sources/${encodeURIComponent(name)}/sync`,
      { method: "POST" }
    );
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || "Failed to trigger arena source sync");
    }
  }

  // ============================================================
  // ArenaConfigs
  // ============================================================

  async getArenaConfigs(workspace: string): Promise<ArenaConfig[]> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/configs`
    );
    if (!response.ok) {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        return [];
      }
      throw new Error(`Failed to fetch arena configs: ${response.statusText}`);
    }
    return response.json();
  }

  async getArenaConfig(workspace: string, name: string): Promise<ArenaConfig | undefined> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/configs/${encodeURIComponent(name)}`
    );
    if (!response.ok) {
      if (response.status === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch arena config: ${response.statusText}`);
    }
    return response.json();
  }

  async getArenaConfigScenarios(workspace: string, name: string): Promise<Scenario[]> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/configs/${encodeURIComponent(name)}/scenarios`
    );
    if (!response.ok) {
      if (response.status === 404) {
        return [];
      }
      throw new Error(`Failed to fetch arena config scenarios: ${response.statusText}`);
    }
    return response.json();
  }

  async createArenaConfig(
    workspace: string,
    name: string,
    spec: ArenaConfigSpec
  ): Promise<ArenaConfig> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/configs`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ metadata: { name }, spec }),
      }
    );
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || "Failed to create arena config");
    }
    return response.json();
  }

  async updateArenaConfig(
    workspace: string,
    name: string,
    spec: ArenaConfigSpec
  ): Promise<ArenaConfig> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/configs/${encodeURIComponent(name)}`,
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ spec }),
      }
    );
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || "Failed to update arena config");
    }
    return response.json();
  }

  async deleteArenaConfig(workspace: string, name: string): Promise<void> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/configs/${encodeURIComponent(name)}`,
      { method: "DELETE" }
    );
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || "Failed to delete arena config");
    }
  }

  // ============================================================
  // ArenaJobs
  // ============================================================

  async getArenaJobs(workspace: string, options?: ArenaJobListOptions): Promise<ArenaJob[]> {
    const params = new URLSearchParams();
    if (options?.type) params.set("type", options.type);
    if (options?.phase) params.set("phase", options.phase);
    if (options?.configRef) params.set("configRef", options.configRef);
    if (options?.limit) params.set("limit", String(options.limit));
    if (options?.sort) params.set("sort", options.sort);

    const queryString = params.toString();
    const suffix = queryString ? `?${queryString}` : "";

    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/jobs${suffix}`
    );
    if (!response.ok) {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        return [];
      }
      throw new Error(`Failed to fetch arena jobs: ${response.statusText}`);
    }
    return response.json();
  }

  async getArenaJob(workspace: string, name: string): Promise<ArenaJob | undefined> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/jobs/${encodeURIComponent(name)}`
    );
    if (!response.ok) {
      if (response.status === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch arena job: ${response.statusText}`);
    }
    return response.json();
  }

  async getArenaJobResults(workspace: string, name: string): Promise<ArenaJobResults | undefined> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/jobs/${encodeURIComponent(name)}/results`
    );
    if (!response.ok) {
      if (response.status === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch arena job results: ${response.statusText}`);
    }
    return response.json();
  }

  async getArenaJobMetrics(workspace: string, name: string): Promise<ArenaJobMetrics | undefined> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/jobs/${encodeURIComponent(name)}/metrics`
    );
    if (!response.ok) {
      if (response.status === 404) {
        return undefined;
      }
      throw new Error(`Failed to fetch arena job metrics: ${response.statusText}`);
    }
    return response.json();
  }

  async createArenaJob(workspace: string, name: string, spec: ArenaJobSpec): Promise<ArenaJob> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/jobs`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ metadata: { name }, spec }),
      }
    );
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || "Failed to create arena job");
    }
    return response.json();
  }

  async cancelArenaJob(workspace: string, name: string): Promise<void> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/jobs/${encodeURIComponent(name)}/cancel`,
      { method: "POST" }
    );
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || "Failed to cancel arena job");
    }
  }

  async deleteArenaJob(workspace: string, name: string): Promise<void> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/jobs/${encodeURIComponent(name)}`,
      { method: "DELETE" }
    );
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || "Failed to delete arena job");
    }
  }

  // ============================================================
  // Stats
  // ============================================================

  async getArenaStats(workspace: string): Promise<ArenaStats> {
    const response = await fetch(
      `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/stats`
    );
    if (!response.ok) {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        return {
          sources: { total: 0, ready: 0, failed: 0, active: 0 },
          configs: { total: 0, ready: 0, scenarios: 0 },
          jobs: { total: 0, running: 0, queued: 0, completed: 0, failed: 0, successRate: 0 },
        };
      }
      throw new Error(`Failed to fetch arena stats: ${response.statusText}`);
    }
    return response.json();
  }
}

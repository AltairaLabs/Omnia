/**
 * Arena API service for Arena Fleet resources.
 *
 * Calls the workspace-scoped Arena API routes:
 *   /api/workspaces/{name}/arena/sources
 *   /api/workspaces/{name}/arena/jobs
 *   /api/workspaces/{name}/arena/stats
 *
 * This service is used by LiveDataService when fetching Arena resources.
 */

import type {
  ArenaSource,
  ArenaSourceSpec,
  ArenaJob,
  ArenaJobSpec,
  ArenaJobResults,
  ArenaJobMetrics,
  ArenaStats,
} from "@/types/arena";
import type { LogEntry, LogOptions } from "./types";

const ARENA_API_BASE = "/api/workspaces";

/** Options for listing Arena jobs */
export interface ArenaJobListOptions {
  /** Filter by job type */
  type?: "evaluation" | "loadtest" | "datagen";
  /** Filter by phase */
  phase?: "Pending" | "Running" | "Succeeded" | "Failed" | "Cancelled";
  /** Filter by source reference */
  sourceRef?: string;
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
  // ArenaJobs
  // ============================================================

  async getArenaJobs(workspace: string, options?: ArenaJobListOptions): Promise<ArenaJob[]> {
    const params = new URLSearchParams();
    if (options?.type) params.set("type", options.type);
    if (options?.phase) params.set("phase", options.phase);
    if (options?.sourceRef) params.set("sourceRef", options.sourceRef);
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

  async getArenaJobLogs(
    workspace: string,
    name: string,
    options?: LogOptions
  ): Promise<LogEntry[]> {
    const params = new URLSearchParams();
    if (options?.tailLines) params.set("tailLines", String(options.tailLines));
    if (options?.sinceSeconds) params.set("sinceSeconds", String(options.sinceSeconds));

    const queryString = params.toString();
    const suffix = queryString ? "?" + queryString : "";
    const url = `${ARENA_API_BASE}/${encodeURIComponent(workspace)}/arena/jobs/${encodeURIComponent(name)}/logs${suffix}`;

    const response = await fetch(url);
    if (!response.ok) {
      if (response.status === 404) {
        return [];
      }
      throw new Error(`Failed to fetch arena job logs: ${response.statusText}`);
    }
    return response.json();
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
          jobs: { total: 0, running: 0, queued: 0, completed: 0, failed: 0, successRate: 0 },
        };
      }
      throw new Error(`Failed to fetch arena stats: ${response.statusText}`);
    }
    return response.json();
  }
}

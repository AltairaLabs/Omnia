/**
 * Memory API service for workspace-scoped memory data.
 *
 * Calls the workspace-scoped memory API proxy routes:
 *   /api/workspaces/{name}/memory
 *   /api/workspaces/{name}/memory/search
 *   /api/workspaces/{name}/memory/export
 *   /api/workspaces/{name}/memory/{memoryId}
 */

import type {
  MemoryEntity,
  MemoryListResponse,
  MemoryListOptions,
  MemorySearchOptions,
} from "./types";

const MEMORY_API_BASE = "/api/workspaces";

/**
 * Memory API service that calls workspace-scoped memory endpoints.
 */
export class MemoryApiService {
  readonly name = "MemoryApiService";

  async getMemories(options: MemoryListOptions): Promise<MemoryListResponse> {
    const { workspace, ...rest } = options;
    const params = new URLSearchParams();
    if (rest.userId) params.set("userId", rest.userId);
    if (rest.type) params.set("type", rest.type);
    if (rest.purpose) params.set("purpose", rest.purpose);
    if (rest.limit !== undefined) params.set("limit", String(rest.limit));
    if (rest.offset !== undefined) params.set("offset", String(rest.offset));

    const queryString = params.toString();
    const suffix = queryString ? `?${queryString}` : "";

    const response = await fetch(
      `${MEMORY_API_BASE}/${encodeURIComponent(workspace)}/memory${suffix}`
    );
    if (!response.ok) {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        return { memories: [], total: 0 };
      }
      throw new Error(`Failed to fetch memories: ${response.statusText}`);
    }

    const data = await response.json();
    return {
      memories: data.memories || [],
      total: data.total ?? 0,
    };
  }

  async searchMemories(options: MemorySearchOptions): Promise<MemoryListResponse> {
    const { workspace, query, minConfidence, ...rest } = options;
    const params = new URLSearchParams();
    params.set("query", query);
    if (rest.userId) params.set("userId", rest.userId);
    if (rest.type) params.set("type", rest.type);
    if (rest.purpose) params.set("purpose", rest.purpose);
    if (rest.limit !== undefined) params.set("limit", String(rest.limit));
    if (rest.offset !== undefined) params.set("offset", String(rest.offset));
    if (minConfidence !== undefined) params.set("minConfidence", String(minConfidence));

    const response = await fetch(
      `${MEMORY_API_BASE}/${encodeURIComponent(workspace)}/memory/search?${params.toString()}`
    );
    if (!response.ok) {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        return { memories: [], total: 0 };
      }
      throw new Error(`Failed to search memories: ${response.statusText}`);
    }

    const data = await response.json();
    return {
      memories: data.memories || [],
      total: data.total ?? 0,
    };
  }

  async exportMemories(workspace: string, userId: string): Promise<MemoryEntity[]> {
    const params = new URLSearchParams({ userId });
    const response = await fetch(
      `${MEMORY_API_BASE}/${encodeURIComponent(workspace)}/memory/export?${params.toString()}`
    );
    if (!response.ok) {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        return [];
      }
      throw new Error(`Failed to export memories: ${response.statusText}`);
    }

    const data = await response.json();
    return data.memories || data || [];
  }

  async deleteMemory(workspace: string, memoryId: string): Promise<void> {
    const response = await fetch(
      `${MEMORY_API_BASE}/${encodeURIComponent(workspace)}/memory/${encodeURIComponent(memoryId)}`,
      { method: "DELETE" }
    );
    if (!response.ok) {
      throw new Error(`Failed to delete memory: ${response.statusText}`);
    }
  }

  async deleteAllMemories(workspace: string, userId: string): Promise<void> {
    const params = new URLSearchParams({ userId });
    const response = await fetch(
      `${MEMORY_API_BASE}/${encodeURIComponent(workspace)}/memory?${params.toString()}`,
      { method: "DELETE" }
    );
    if (!response.ok) {
      throw new Error(`Failed to delete all memories: ${response.statusText}`);
    }
  }
}

/**
 * Institutional memory service for workspace-scoped operator-curated memories.
 *
 * Calls the workspace-scoped institutional memory proxy routes:
 *   /api/workspaces/{name}/institutional-memory
 *   /api/workspaces/{name}/institutional-memory/{memoryId}
 */

import type { MemoryEntity, MemoryListResponse } from "./types";

const API_BASE = "/api/workspaces";

export interface InstitutionalListOptions {
  workspace: string;
  limit?: number;
  offset?: number;
}

export interface InstitutionalCreateInput {
  workspace: string;
  type: string;
  content: string;
  confidence?: number;
  metadata?: Record<string, unknown>;
}

/**
 * Service for operator-curated workspace memory CRUD.
 */
export class InstitutionalMemoryService {
  readonly name = "InstitutionalMemoryService";

  async list(options: InstitutionalListOptions): Promise<MemoryListResponse> {
    const { workspace, limit, offset } = options;
    const params = new URLSearchParams();
    if (limit !== undefined) params.set("limit", String(limit));
    if (offset !== undefined) params.set("offset", String(offset));

    const qs = params.toString();
    const suffix = qs ? `?${qs}` : "";
    const response = await fetch(
      `${API_BASE}/${encodeURIComponent(workspace)}/institutional-memory${suffix}`
    );
    if (!response.ok) {
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        return { memories: [], total: 0 };
      }
      throw new Error(`Failed to list institutional memories: ${response.statusText}`);
    }
    const data = await response.json();
    return {
      memories: data.memories || [],
      total: data.total ?? 0,
    };
  }

  async create(input: InstitutionalCreateInput): Promise<MemoryEntity> {
    const { workspace, ...body } = input;
    const response = await fetch(
      `${API_BASE}/${encodeURIComponent(workspace)}/institutional-memory`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          type: body.type,
          content: body.content,
          confidence: body.confidence ?? 1.0,
          metadata: body.metadata ?? {},
        }),
      }
    );
    if (!response.ok) {
      const err = await response.json().catch(() => ({ error: response.statusText }));
      throw new Error(`Failed to create institutional memory: ${err.error ?? response.statusText}`);
    }
    const data = await response.json();
    return data.memory as MemoryEntity;
  }

  async delete(workspace: string, memoryId: string): Promise<void> {
    const response = await fetch(
      `${API_BASE}/${encodeURIComponent(workspace)}/institutional-memory/${encodeURIComponent(memoryId)}`,
      { method: "DELETE" }
    );
    if (!response.ok) {
      const err = await response.json().catch(() => ({ error: response.statusText }));
      throw new Error(`Failed to delete institutional memory: ${err.error ?? response.statusText}`);
    }
  }
}

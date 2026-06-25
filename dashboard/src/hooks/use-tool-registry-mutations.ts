"use client";

import { useCallback, useState } from "react";
import { useWorkspace } from "@/contexts/workspace-context";
import type { ToolRegistry, ToolRegistrySpec } from "@/types/tool-registry";

const NO_WORKSPACE_ERROR = "No workspace selected";

/** Body shape for a full-replace update of a CRD resource via the item PUT route. */
export interface UpdateResourceBody {
  metadata: {
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    resourceVersion?: string;
  };
  spec: unknown;
}

/** Error thrown when a resource update is rejected, carrying the HTTP status. */
export class ResourceUpdateError extends Error {
  constructor(public readonly status: number, message: string) {
    super(message);
    this.name = "ResourceUpdateError";
  }
}

export interface UseToolRegistryMutationsResult {
  createToolRegistry: (name: string, spec: ToolRegistrySpec) => Promise<ToolRegistry>;
  updateToolRegistry: (name: string, body: UpdateResourceBody) => Promise<ToolRegistry>;
  loading: boolean;
  error: Error | null;
}

/**
 * useToolRegistryMutations provides create for workspace-scoped ToolRegistries,
 * mirroring useProviderMutations. Persists via POST
 * /api/workspaces/:name/toolregistries (the collection route's factory-generated
 * POST handler).
 */
export function useToolRegistryMutations(): UseToolRegistryMutationsResult {
  const { currentWorkspace } = useWorkspace();
  const workspace = currentWorkspace?.name;
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);

  const createToolRegistry = useCallback(
    async (name: string, spec: ToolRegistrySpec): Promise<ToolRegistry> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(`/api/workspaces/${workspace}/toolregistries`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ metadata: { name }, spec }),
        });

        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || "Failed to create tool registry");
        }

        return response.json();
      } catch (err) {
        const e = err instanceof Error ? err : new Error(String(err));
        setError(e);
        throw e;
      } finally {
        setLoading(false);
      }
    },
    [workspace]
  );

  const updateToolRegistry = useCallback(
    async (name: string, body: UpdateResourceBody): Promise<ToolRegistry> => {
      if (!workspace) {
        throw new Error(NO_WORKSPACE_ERROR);
      }

      setLoading(true);
      setError(null);

      try {
        const response = await fetch(`/api/workspaces/${workspace}/toolregistries/${name}`, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        });

        if (!response.ok) {
          const data = await response.json().catch(() => ({}));
          throw new ResourceUpdateError(
            response.status,
            data.message || "Failed to update tool registry"
          );
        }

        return response.json();
      } catch (err) {
        const e = err instanceof Error ? err : new Error(String(err));
        setError(e);
        throw e;
      } finally {
        setLoading(false);
      }
    },
    [workspace]
  );

  return { createToolRegistry, updateToolRegistry, loading, error };
}

"use client";

import { useQuery } from "@tanstack/react-query";
import type { ToolRegistry } from "@/types";

/**
 * Fetches shared tool registries from the system namespace.
 * Shared tool registries are available to all workspaces (read-only).
 */
export function useSharedToolRegistries() {
  return useQuery({
    queryKey: ["shared-tool-registries"],
    queryFn: async (): Promise<ToolRegistry[]> => {
      const response = await fetch("/api/shared/toolregistries");
      if (!response.ok) {
        if (response.status === 401) {
          return []; // Not authenticated, return empty
        }
        throw new Error(`Failed to fetch shared tool registries: ${response.statusText}`);
      }
      return response.json();
    },
  });
}

/**
 * Fetches a single shared tool registry by name.
 */
export function useSharedToolRegistry(name: string) {
  return useQuery({
    queryKey: ["shared-tool-registry", name],
    queryFn: async (): Promise<ToolRegistry | null> => {
      const response = await fetch(`/api/shared/toolregistries/${encodeURIComponent(name)}`);
      if (!response.ok) {
        if (response.status === 401 || response.status === 404) {
          return null;
        }
        throw new Error(`Failed to fetch shared tool registry: ${response.statusText}`);
      }
      return response.json();
    },
    enabled: !!name,
  });
}

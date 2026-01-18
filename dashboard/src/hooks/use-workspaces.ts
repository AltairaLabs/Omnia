"use client";

/**
 * Hook for fetching user's accessible workspaces.
 * Uses React Query for caching and automatic refetching.
 */

import { useQuery } from "@tanstack/react-query";
import type { WorkspaceRole, WorkspacePermissions } from "@/types/workspace";

/**
 * Workspace item returned from the API.
 */
export interface WorkspaceListItem {
  name: string;
  displayName: string;
  description?: string;
  environment: "development" | "staging" | "production";
  namespace: string;
  role: WorkspaceRole;
  permissions: WorkspacePermissions;
  createdAt?: string;
}

/**
 * Response from /api/workspaces endpoint.
 */
interface WorkspacesResponse {
  workspaces: WorkspaceListItem[];
  count: number;
}

/**
 * Options for fetching workspaces.
 */
interface UseWorkspacesOptions {
  /** Minimum role required (filters workspaces) */
  minRole?: WorkspaceRole;
  /** Whether to enable the query */
  enabled?: boolean;
}

/**
 * Fetch workspaces from the API.
 */
async function fetchWorkspaces(minRole?: WorkspaceRole): Promise<WorkspaceListItem[]> {
  const url = minRole ? `/api/workspaces?minRole=${minRole}` : "/api/workspaces";
  const response = await fetch(url);

  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: "Failed to fetch workspaces" }));
    throw new Error(error.message || "Failed to fetch workspaces");
  }

  const data: WorkspacesResponse = await response.json();
  return data.workspaces;
}

/**
 * Hook to fetch workspaces the current user has access to.
 *
 * @example
 * ```tsx
 * function MyComponent() {
 *   const { data: workspaces, isLoading, error } = useWorkspaces();
 *
 *   if (isLoading) return <Spinner />;
 *   if (error) return <ErrorMessage error={error} />;
 *
 *   return (
 *     <ul>
 *       {workspaces?.map(ws => (
 *         <li key={ws.name}>{ws.displayName} ({ws.role})</li>
 *       ))}
 *     </ul>
 *   );
 * }
 * ```
 */
export function useWorkspaces(options: UseWorkspacesOptions = {}) {
  const { minRole, enabled = true } = options;

  return useQuery({
    queryKey: ["workspaces", minRole],
    queryFn: () => fetchWorkspaces(minRole),
    staleTime: 60000, // Cache for 1 minute
    enabled,
  });
}

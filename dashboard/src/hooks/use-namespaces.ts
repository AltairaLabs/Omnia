"use client";

import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";

/**
 * @deprecated Use useWorkspace() from workspace-context instead.
 * Namespaces are now derived from workspaces.
 *
 * Returns unique namespaces from available workspaces for backward compatibility.
 */
export function useNamespaces() {
  const { workspaces } = useWorkspace();

  return useQuery<string[]>({
    queryKey: ["namespaces", workspaces],
    queryFn: (): string[] => {
      // Derive unique namespaces from workspaces
      const namespaces = new Set<string>();
      workspaces.forEach((ws) => {
        if (ws.namespace) {
          namespaces.add(ws.namespace);
        }
      });
      return Array.from(namespaces);
    },
    staleTime: 60000, // Cache for 1 minute
  });
}

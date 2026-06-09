"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "@/contexts/workspace-context";
import { SessionApiService, type SessionPurgeScope } from "@/lib/data/session-api-service";

const service = new SessionApiService();

/** Invalidate every session-scoped query so list/detail views refresh. */
function invalidateSessionQueries(queryClient: ReturnType<typeof useQueryClient>) {
  for (const key of ["sessions", "sessions-search", "session", "session-all-messages"]) {
    queryClient.invalidateQueries({ queryKey: [key] });
  }
}

/**
 * Delete a single session by ID (requires editor role on the workspace).
 * Invalidates session query caches on success.
 */
export function useDeleteSession() {
  const { currentWorkspace } = useWorkspace();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (sessionId: string) => {
      if (!currentWorkspace) throw new Error("No workspace selected");
      return service.deleteSession(currentWorkspace.name, sessionId);
    },
    onSuccess: () => invalidateSessionQueries(queryClient),
  });
}

/**
 * Bulk-purge sessions in the workspace (requires owner role). Optionally
 * narrowed by agent and/or a before-cutoff. User-agnostic — removes
 * automated sessions too. Returns the number deleted.
 */
export function usePurgeSessions() {
  const { currentWorkspace } = useWorkspace();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (scope?: SessionPurgeScope) => {
      if (!currentWorkspace) throw new Error("No workspace selected");
      return service.purgeSessions(currentWorkspace.name, scope ?? {});
    },
    onSuccess: () => invalidateSessionQueries(queryClient),
  });
}

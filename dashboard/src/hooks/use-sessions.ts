"use client";

import { useQuery } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";
import { useWorkspace } from "@/contexts/workspace-context";
import type {
  SessionListOptions,
  SessionSearchOptions,
  SessionMessageOptions,
} from "@/types/session";

/**
 * Fetch sessions for the current workspace with optional filters.
 * Auto-refreshes every 10s when active sessions may exist.
 */
export function useSessions(options: SessionListOptions = {}) {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();

  const { agent, status, from, to, limit, offset } = options;

  const query = useQuery({
    queryKey: ["sessions", currentWorkspace?.name, agent, status, from, to, limit, offset, service.name],
    queryFn: async () => {
      if (!currentWorkspace) {
        return { sessions: [], total: 0, hasMore: false };
      }
      return service.getSessions(currentWorkspace.name, options);
    },
    enabled: !!currentWorkspace,
    staleTime: 10000,
    refetchInterval: (query) => {
      // Auto-refresh when there might be active sessions
      const data = query.state.data;
      if (data?.sessions.some((s) => s.status === "active")) {
        return 10000;
      }
      return false;
    },
  });

  return query;
}

/**
 * Fetch a single session by ID.
 * Auto-refreshes every 5s when the session is active.
 */
export function useSessionDetail(sessionId: string) {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();

  return useQuery({
    queryKey: ["session", currentWorkspace?.name, sessionId, service.name],
    queryFn: async () => {
      if (!currentWorkspace) {
        return undefined;
      }
      return service.getSessionById(currentWorkspace.name, sessionId);
    },
    enabled: !!currentWorkspace && !!sessionId,
    staleTime: 5000,
    refetchInterval: (query) => {
      const data = query.state.data;
      if (data?.status === "active") {
        return 5000;
      }
      return false;
    },
  });
}

/**
 * Search sessions with a debounced query string.
 * Only fires when q is non-empty.
 */
export function useSessionSearch(options: SessionSearchOptions) {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();

  const { q, agent, status, from, to, limit, offset } = options;

  return useQuery({
    queryKey: ["sessions-search", currentWorkspace?.name, q, agent, status, from, to, limit, offset, service.name],
    queryFn: async () => {
      if (!currentWorkspace) {
        return { sessions: [], total: 0, hasMore: false };
      }
      return service.searchSessions(currentWorkspace.name, options);
    },
    enabled: !!currentWorkspace && !!q,
    staleTime: 10000,
  });
}

/**
 * Fetch paginated messages for a session.
 */
export function useSessionMessages(sessionId: string, options: SessionMessageOptions = {}) {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();

  const { limit, before, after } = options;

  return useQuery({
    queryKey: ["session-messages", currentWorkspace?.name, sessionId, limit, before, after, service.name],
    queryFn: async () => {
      if (!currentWorkspace) {
        return { messages: [], hasMore: false };
      }
      return service.getSessionMessages(currentWorkspace.name, sessionId, options);
    },
    enabled: !!currentWorkspace && !!sessionId,
    staleTime: 5000,
  });
}

"use client";

import { useMemo } from "react";
import { useQuery, useInfiniteQuery } from "@tanstack/react-query";
import { useDataService } from "@/lib/data";
import { SessionApiService } from "@/lib/data/session-api-service";
import { useWorkspace } from "@/contexts/workspace-context";
import type {
  SessionListOptions,
  SessionSearchOptions,
  SessionMessageOptions,
  Message,
  ToolCall,
  ProviderCall,
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

/** Page size for infinite message loading. */
const MESSAGE_PAGE_SIZE = 100;

/**
 * Fetch all session messages using cursor-based infinite loading.
 * Pages are fetched via the /messages endpoint using `after` sequence cursors.
 * Returns a flat deduplicated message array and load-more controls.
 */
export function useSessionAllMessages(sessionId: string, enabled = true) {
  const service = useDataService();
  const { currentWorkspace } = useWorkspace();

  const query = useInfiniteQuery({
    queryKey: ["session-all-messages", currentWorkspace?.name, sessionId, service.name],
    queryFn: async ({ pageParam }) => {
      if (!currentWorkspace) {
        return { messages: [] as Message[], hasMore: false };
      }
      const opts: SessionMessageOptions = { limit: MESSAGE_PAGE_SIZE };
      if (pageParam !== undefined) {
        opts.after = pageParam;
      }
      return service.getSessionMessages(currentWorkspace.name, sessionId, opts);
    },
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (lastPage) => {
      if (!lastPage.hasMore || lastPage.messages.length === 0) return undefined;
      const lastMsg = lastPage.messages[lastPage.messages.length - 1];
      return lastMsg.sequenceNum;
    },
    enabled: !!currentWorkspace && !!sessionId && enabled,
    staleTime: 5000,
    refetchInterval: (query) => {
      // Only auto-refetch if we haven't loaded all pages yet
      // (active sessions keep getting new messages)
      const pages = query.state.data?.pages;
      if (pages && pages.length > 0) {
        const lastPage = pages[pages.length - 1];
        if (lastPage.hasMore) return false; // don't refetch partial loads
      }
      return 5000;
    },
  });

  // Flatten pages into a single deduplicated message array
  const pages = query.data?.pages;
  const messages = useMemo(() => {
    if (!pages) return [];
    const seen = new Set<string>();
    const result: Message[] = [];
    for (const page of pages) {
      for (const msg of page.messages) {
        if (!seen.has(msg.id)) {
          seen.add(msg.id);
          result.push(msg);
        }
      }
    }
    return result;
  }, [pages]);

  const totalLoaded = messages.length;

  return {
    messages,
    totalLoaded,
    hasMore: query.hasNextPage ?? false,
    isLoading: query.isLoading,
    isFetchingMore: query.isFetchingNextPage,
    fetchMore: query.fetchNextPage,
    error: query.error,
  };
}

/**
 * Fetch tool calls for a session from the first-class tool_calls table.
 */
export function useSessionToolCalls(sessionId: string) {
  const { currentWorkspace } = useWorkspace();

  return useQuery({
    queryKey: ["session-tool-calls", currentWorkspace?.name, sessionId],
    queryFn: async (): Promise<ToolCall[]> => {
      if (!currentWorkspace) return [];
      const service = new SessionApiService();
      return service.getToolCalls(currentWorkspace.name, sessionId);
    },
    enabled: !!currentWorkspace && !!sessionId,
    staleTime: 10000,
  });
}

/**
 * Fetch provider calls for a session from the first-class provider_calls table.
 */
export function useSessionProviderCalls(sessionId: string) {
  const { currentWorkspace } = useWorkspace();

  return useQuery({
    queryKey: ["session-provider-calls", currentWorkspace?.name, sessionId],
    queryFn: async (): Promise<ProviderCall[]> => {
      if (!currentWorkspace) return [];
      const service = new SessionApiService();
      return service.getProviderCalls(currentWorkspace.name, sessionId);
    },
    enabled: !!currentWorkspace && !!sessionId,
    staleTime: 10000,
  });
}

/**
 * Fetch eval results for a session.
 * Uses SessionApiService directly since eval results are not part of the
 * DataService interface (they are session-api specific).
 */
export function useSessionEvalResults(sessionId: string) {
  const { currentWorkspace } = useWorkspace();

  return useQuery({
    queryKey: ["session-eval-results", currentWorkspace?.name, sessionId],
    queryFn: async () => {
      if (!currentWorkspace) {
        return [];
      }
      const service = new SessionApiService();
      return service.getSessionEvalResults(currentWorkspace.name, sessionId);
    },
    enabled: !!currentWorkspace && !!sessionId,
    staleTime: 10000,
  });
}

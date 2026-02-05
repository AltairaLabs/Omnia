/**
 * React hook for managing Arena dev sessions.
 *
 * Provides methods to create, get, update, and delete dev sessions,
 * as well as establish WebSocket connections to the dev console.
 */

import { useState, useCallback, useEffect, useRef } from "react";
import useSWR from "swr";
import type { ArenaDevSession } from "@/types/arena";

const API_BASE = "/api/workspaces";

interface UseDevSessionOptions {
  workspace: string;
  projectId: string;
  /** Polling interval when waiting for session to be ready (ms) */
  pollingInterval?: number;
  /** Auto-create session if one doesn't exist */
  autoCreate?: boolean;
}

interface UseDevSessionResult {
  /** Current session (if any) */
  session: ArenaDevSession | null;
  /** Whether a session is being created or loaded */
  isLoading: boolean;
  /** Error from session operations */
  error: Error | null;
  /** Whether the session is ready for WebSocket connection */
  isReady: boolean;
  /** WebSocket endpoint URL (when ready) */
  endpoint: string | null;
  /** Create a new dev session */
  createSession: (options?: { idleTimeout?: string }) => Promise<ArenaDevSession>;
  /** Delete the current session */
  deleteSession: () => Promise<void>;
  /** Send a heartbeat to keep the session alive */
  sendHeartbeat: () => Promise<void>;
  /** Refresh session data */
  refresh: () => Promise<void>;
}

const fetcher = async (url: string) => {
  const response = await fetch(url);
  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: "Unknown error" }));
    throw new Error(error.message || `HTTP ${response.status}`);
  }
  return response.json();
};

/**
 * Hook to manage an Arena dev session for interactive testing.
 */
export function useDevSession({
  workspace,
  projectId,
  pollingInterval = 5000, // Increased from 2000ms to reduce API call frequency
  autoCreate = false,
}: UseDevSessionOptions): UseDevSessionResult {
  const [isCreating, setIsCreating] = useState(false);
  const [createError, setCreateError] = useState<Error | null>(null);
  const heartbeatRef = useRef<NodeJS.Timeout | null>(null);

  // Fetch sessions for this project
  const {
    data: sessions,
    error: fetchError,
    isLoading: isFetching,
    mutate,
  } = useSWR<ArenaDevSession[]>(
    workspace && projectId
      ? `${API_BASE}/${workspace}/arena/dev-sessions?projectId=${projectId}`
      : null,
    fetcher,
    {
      // Poll when waiting for session to be ready
      refreshInterval: (data) => {
        const activeSession = data?.find(
          (s) => s.status?.phase === "Pending" || s.status?.phase === "Starting"
        );
        return activeSession ? pollingInterval : 0;
      },
    }
  );

  // Find the active session (Ready, Starting, or Pending)
  const session =
    sessions?.find(
      (s) =>
        s.status?.phase === "Ready" ||
        s.status?.phase === "Starting" ||
        s.status?.phase === "Pending"
    ) || null;

  const isReady = session?.status?.phase === "Ready";
  const endpoint = isReady ? session?.status?.endpoint || null : null;

  // Create a new session
  const createSession = useCallback(
    async (options?: { idleTimeout?: string }): Promise<ArenaDevSession> => {
      setIsCreating(true);
      setCreateError(null);

      try {
        const response = await fetch(`${API_BASE}/${workspace}/arena/dev-sessions`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            projectId,
            idleTimeout: options?.idleTimeout || "30m",
          }),
        });

        if (!response.ok) {
          const error = await response.json().catch(() => ({ message: "Failed to create session" }));
          throw new Error(error.message);
        }

        const created = await response.json();
        await mutate();
        return created;
      } catch (err) {
        const error = err instanceof Error ? err : new Error(String(err));
        setCreateError(error);
        throw error;
      } finally {
        setIsCreating(false);
      }
    },
    [workspace, projectId, mutate]
  );

  // Delete the current session
  const deleteSession = useCallback(async (): Promise<void> => {
    if (!session) return;

    const response = await fetch(
      `${API_BASE}/${workspace}/arena/dev-sessions/${session.metadata.name}`,
      { method: "DELETE" }
    );

    if (!response.ok && response.status !== 404) {
      const error = await response.json().catch(() => ({ message: "Failed to delete session" }));
      throw new Error(error.message);
    }

    await mutate();
  }, [workspace, session, mutate]);

  // Send heartbeat to keep session alive
  const sendHeartbeat = useCallback(async (): Promise<void> => {
    if (!session || !isReady) return;

    try {
      await fetch(`${API_BASE}/${workspace}/arena/dev-sessions/${session.metadata.name}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          status: {
            lastActivityAt: new Date().toISOString(),
          },
        }),
      });
    } catch {
      // Heartbeat failures are non-fatal
      console.warn("Failed to send heartbeat");
    }
  }, [workspace, session, isReady]);

  // Refresh session data
  const refresh = useCallback(async (): Promise<void> => {
    await mutate();
  }, [mutate]);

  // Auto-create session if requested
  useEffect(() => {
    if (autoCreate && !session && !isFetching && !isCreating && sessions !== undefined) {
      createSession().catch(() => {
        // Error is already captured in createError
      });
    }
  }, [autoCreate, session, isFetching, isCreating, sessions, createSession]);

  // Set up heartbeat interval when session is ready
  useEffect(() => {
    if (isReady) {
      // Send heartbeat every 5 minutes
      heartbeatRef.current = setInterval(sendHeartbeat, 5 * 60 * 1000);
      return () => {
        if (heartbeatRef.current) {
          clearInterval(heartbeatRef.current);
        }
      };
    }
  }, [isReady, sendHeartbeat]);

  return {
    session,
    isLoading: isFetching || isCreating,
    error: fetchError || createError,
    isReady,
    endpoint,
    createSession,
    deleteSession,
    sendHeartbeat,
    refresh,
  };
}

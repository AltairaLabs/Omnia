"use client";

import { useMemo } from "react";
import { useSessions } from "./use-sessions";

export interface AdjacentSessions {
  /** Session id immediately above the current one in the list (newer). */
  readonly prevId: string | null;
  /** Session id immediately below the current one in the list (older). */
  readonly nextId: string | null;
  /** 1-based position of the current session, or null if not in the list. */
  readonly position: number | null;
  /** Total sessions in the loaded list. */
  readonly total: number;
}

/**
 * Resolve the previous / next session ids relative to `currentId` using the
 * same default session list the list page shows. If the current session is
 * not in the list (filter / pagination / archived), returns null both ways
 * so callers can hide the nav rather than point to something stale.
 */
export function useAdjacentSessions(currentId: string | undefined): AdjacentSessions {
  const { data } = useSessions();

  return useMemo(() => {
    const sessions = data?.sessions ?? [];
    if (!currentId || sessions.length === 0) {
      return { prevId: null, nextId: null, position: null, total: sessions.length };
    }
    const idx = sessions.findIndex((s) => s.id === currentId);
    if (idx === -1) {
      return { prevId: null, nextId: null, position: null, total: sessions.length };
    }
    return {
      prevId: idx > 0 ? sessions[idx - 1].id : null,
      nextId: idx < sessions.length - 1 ? sessions[idx + 1].id : null,
      position: idx + 1,
      total: sessions.length,
    };
  }, [data?.sessions, currentId]);
}

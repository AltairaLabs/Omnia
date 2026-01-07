"use client";

import { useCallback, useSyncExternalStore } from "react";
import type { ConsoleMessage, ConsoleState, ConnectionStatus } from "@/types/websocket";

/**
 * Simple store for console state that persists across component unmounts.
 * Uses a Map keyed by "namespace/agentName" to store state for each agent.
 */

type ConsoleKey = string;

interface StoredConsoleState extends ConsoleState {
  // Additional metadata
  lastActivity: Date;
}

// Module-level store that persists across component lifecycles
const consoleStates = new Map<ConsoleKey, StoredConsoleState>();
const listeners = new Set<() => void>();

function getKey(namespace: string, agentName: string): ConsoleKey {
  return `${namespace}/${agentName}`;
}

function getState(key: ConsoleKey): StoredConsoleState {
  const existing = consoleStates.get(key);
  if (existing) return existing;

  // Return default state
  return {
    sessionId: null,
    status: "disconnected",
    messages: [],
    error: null,
    lastActivity: new Date(),
  };
}

function setState(key: ConsoleKey, state: Partial<StoredConsoleState>): void {
  const existing = getState(key);
  consoleStates.set(key, {
    ...existing,
    ...state,
    lastActivity: new Date(),
  });
  // Notify all subscribers
  listeners.forEach((listener) => listener());
}

function subscribe(callback: () => void): () => void {
  listeners.add(callback);
  return () => listeners.delete(callback);
}

/**
 * Hook to access and update console state for an agent.
 * State persists even when the component unmounts.
 */
export function useConsoleStore(namespace: string, agentName: string) {
  const key = getKey(namespace, agentName);

  // Subscribe to store changes
  const store = useSyncExternalStore(
    subscribe,
    () => getState(key),
    () => getState(key)
  );

  const setMessages = useCallback(
    (messages: ConsoleMessage[]) => {
      setState(key, { messages });
    },
    [key]
  );

  const addMessage = useCallback(
    (message: ConsoleMessage) => {
      const current = getState(key);
      setState(key, { messages: [...current.messages, message] });
    },
    [key]
  );

  const updateLastMessage = useCallback(
    (updater: (msg: ConsoleMessage) => ConsoleMessage) => {
      const current = getState(key);
      if (current.messages.length === 0) return;

      const messages = [...current.messages];
      messages[messages.length - 1] = updater(messages[messages.length - 1]);
      setState(key, { messages });
    },
    [key]
  );

  const setStatus = useCallback(
    (status: ConnectionStatus, error?: string | null) => {
      setState(key, { status, error: error ?? null });
    },
    [key]
  );

  const setSessionId = useCallback(
    (sessionId: string | null) => {
      setState(key, { sessionId });
    },
    [key]
  );

  const clearMessages = useCallback(() => {
    setState(key, { messages: [], sessionId: null });
  }, [key]);

  return {
    ...store,
    setMessages,
    addMessage,
    updateLastMessage,
    setStatus,
    setSessionId,
    clearMessages,
  };
}

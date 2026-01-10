"use client";

import { useCallback, useSyncExternalStore } from "react";
import type { ConsoleMessage, ConsoleState, ConnectionStatus } from "@/types/websocket";

/**
 * Simple store for console state that persists across component unmounts.
 * Uses a Map keyed by session ID or "namespace/agentName" to store state.
 *
 * Two APIs are provided:
 * - useConsoleStoreBySession(sessionId): Use a custom session ID as the key
 * - useConsoleStore(namespace, agentName): Legacy API using namespace/agentName as key
 */

interface StoredConsoleState extends ConsoleState {
  // Additional metadata
  lastActivity: Date;
}

// Module-level store that persists across component lifecycles
const consoleStates = new Map<string, StoredConsoleState>();
const listeners = new Set<() => void>();

// Cache for snapshot references to prevent useSyncExternalStore infinite loop
// The snapshot function must return a cached value when state hasn't changed
const snapshotCache = new Map<string, StoredConsoleState>();

function getKey(namespace: string, agentName: string): string {
  return `${namespace}/${agentName}`;
}

function createDefaultState(): StoredConsoleState {
  return {
    sessionId: null,
    status: "disconnected",
    messages: [],
    error: null,
    lastActivity: new Date(),
  };
}

function getState(key: string): StoredConsoleState {
  const existing = consoleStates.get(key);
  if (existing) return existing;

  // Return cached default state to satisfy useSyncExternalStore's caching requirement
  let cached = snapshotCache.get(key);
  if (!cached) {
    cached = createDefaultState();
    snapshotCache.set(key, cached);
  }
  return cached;
}

function setState(key: string, state: Partial<StoredConsoleState>): void {
  const existing = getState(key);
  consoleStates.set(key, {
    ...existing,
    ...state,
    lastActivity: new Date(),
  });
  // Clear the default state cache since we now have real state
  snapshotCache.delete(key);
  // Notify all subscribers
  listeners.forEach((listener) => listener());
}

function subscribe(callback: () => void): () => void {
  listeners.add(callback);
  return () => listeners.delete(callback);
}

/**
 * Clear state for a given key. Used when closing tabs.
 */
export function clearConsoleState(key: string): void {
  consoleStates.delete(key);
  snapshotCache.delete(key);
  listeners.forEach((listener) => listener());
}

/**
 * Hook to access and update console state using a session ID as the key.
 * Use this for multi-session tab support where each tab has a unique session ID.
 * State persists even when the component unmounts.
 */
export function useConsoleStoreBySession(sessionKey: string) {
  // Subscribe to store changes
  const store = useSyncExternalStore(
    subscribe,
    () => getState(sessionKey),
    () => getState(sessionKey)
  );

  const setMessages = useCallback(
    (messages: ConsoleMessage[]) => {
      setState(sessionKey, { messages });
    },
    [sessionKey]
  );

  const addMessage = useCallback(
    (message: ConsoleMessage) => {
      const current = getState(sessionKey);
      setState(sessionKey, { messages: [...current.messages, message] });
    },
    [sessionKey]
  );

  const updateLastMessage = useCallback(
    (updater: (msg: ConsoleMessage) => ConsoleMessage) => {
      const current = getState(sessionKey);
      if (current.messages.length === 0) return;

      const messages = [...current.messages];
      const lastMessage = messages.at(-1);
      if (lastMessage) {
        messages[messages.length - 1] = updater(lastMessage);
      }
      setState(sessionKey, { messages });
    },
    [sessionKey]
  );

  const setStatus = useCallback(
    (status: ConnectionStatus, error?: string | null) => {
      setState(sessionKey, { status, error: error ?? null });
    },
    [sessionKey]
  );

  const setSessionId = useCallback(
    (sessionId: string | null) => {
      setState(sessionKey, { sessionId });
    },
    [sessionKey]
  );

  const clearMessages = useCallback(() => {
    setState(sessionKey, { messages: [], sessionId: null });
  }, [sessionKey]);

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

/**
 * Hook to access and update console state for an agent.
 * Uses namespace/agentName as the key. For multi-session support,
 * use useConsoleStoreBySession instead.
 * State persists even when the component unmounts.
 */
export function useConsoleStore(namespace: string, agentName: string) {
  const key = getKey(namespace, agentName);
  return useConsoleStoreBySession(key);
}

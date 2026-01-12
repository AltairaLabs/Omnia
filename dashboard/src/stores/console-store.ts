"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { ConsoleMessage, ConnectionStatus } from "@/types/websocket";

// ============================================================
// Types
// ============================================================

export interface ConsoleTab {
  /** Unique identifier for the tab - also used as the session key */
  id: string;
  /** Current state of the tab */
  state: "selecting" | "active";
  /** Agent name (set when agent is selected) */
  agentName?: string;
  /** Agent namespace (set when agent is selected) */
  namespace?: string;
  /** Timestamp when the tab was created */
  createdAt: number;
}

export interface SessionState {
  sessionId: string | null;
  status: ConnectionStatus;
  messages: ConsoleMessage[];
  error: string | null;
  lastActivity: number;
}

interface ConsoleStoreState {
  // Tab state
  tabs: ConsoleTab[];
  activeTabId: string | null;

  // Session state per tab (keyed by tab.id)
  sessions: Record<string, SessionState>;
}

interface ConsoleStoreActions {
  // Tab actions
  createTab: () => string;
  closeTab: (id: string) => void;
  setActiveTab: (id: string) => void;
  updateTab: (id: string, updates: Partial<Omit<ConsoleTab, "id">>) => void;
  clearAllTabs: () => void;

  // Session actions
  setSessionId: (tabId: string, sessionId: string | null) => void;
  setStatus: (tabId: string, status: ConnectionStatus, error?: string | null) => void;
  setMessages: (tabId: string, messages: ConsoleMessage[]) => void;
  addMessage: (tabId: string, message: ConsoleMessage) => void;
  updateLastMessage: (tabId: string, updater: (msg: ConsoleMessage) => ConsoleMessage) => void;
  clearMessages: (tabId: string) => void;
}

export type ConsoleStore = ConsoleStoreState & ConsoleStoreActions;

// ============================================================
// Constants
// ============================================================

const MAX_TABS = 10;
const STORAGE_KEY = "omnia-console";

// ============================================================
// Helpers
// ============================================================

function generateTabId(): string {
  return `tab-${Date.now()}-${crypto.randomUUID().slice(0, 8)}`;
}

function createDefaultSession(): SessionState {
  return {
    sessionId: null,
    status: "disconnected",
    messages: [],
    error: null,
    lastActivity: Date.now(),
  };
}

function getSession(sessions: Record<string, SessionState>, tabId: string): SessionState {
  return sessions[tabId] || createDefaultSession();
}

// ============================================================
// Store
// ============================================================

export const useConsoleStore = create<ConsoleStore>()(
  persist(
    (set, get) => ({
      // Initial state
      tabs: [],
      activeTabId: null,
      sessions: {},

      // Tab actions
      createTab: () => {
        const state = get();

        // Enforce max tab limit - remove oldest inactive tab if at limit
        let tabs = state.tabs;
        if (tabs.length >= MAX_TABS) {
          const oldestInactive = tabs
            .filter((t) => t.id !== state.activeTabId)
            .sort((a, b) => a.createdAt - b.createdAt)[0];

          if (oldestInactive) {
            const remainingSessions = Object.fromEntries(
              Object.entries(state.sessions).filter(([key]) => key !== oldestInactive.id)
            );
            tabs = tabs.filter((t) => t.id !== oldestInactive.id);
            set({ sessions: remainingSessions });
          }
        }

        const newTab: ConsoleTab = {
          id: generateTabId(),
          state: "selecting",
          createdAt: Date.now(),
        };

        set({
          tabs: [...tabs, newTab],
          activeTabId: newTab.id,
        });

        return newTab.id;
      },

      closeTab: (id) => {
        const state = get();
        const tabIndex = state.tabs.findIndex((t) => t.id === id);
        if (tabIndex === -1) return;

        const newTabs = state.tabs.filter((t) => t.id !== id);
        let newActiveId = state.activeTabId;

        // If closing active tab, select adjacent one
        if (state.activeTabId === id) {
          if (newTabs.length > 0) {
            const nextIndex = Math.min(tabIndex, newTabs.length - 1);
            newActiveId = newTabs[nextIndex].id;
          } else {
            newActiveId = null;
          }
        }

        // Remove session state for closed tab
        const remainingSessions = Object.fromEntries(
          Object.entries(state.sessions).filter(([key]) => key !== id)
        );

        set({
          tabs: newTabs,
          activeTabId: newActiveId,
          sessions: remainingSessions,
        });
      },

      setActiveTab: (id) => {
        const state = get();
        if (!state.tabs.find((t) => t.id === id)) return;
        if (state.activeTabId === id) return;

        set({ activeTabId: id });
      },

      updateTab: (id, updates) => {
        const state = get();
        const tabIndex = state.tabs.findIndex((t) => t.id === id);
        if (tabIndex === -1) return;

        const newTabs = [...state.tabs];
        newTabs[tabIndex] = { ...newTabs[tabIndex], ...updates };

        set({ tabs: newTabs });
      },

      clearAllTabs: () => {
        set({
          tabs: [],
          activeTabId: null,
          sessions: {},
        });
      },

      // Session actions
      setSessionId: (tabId, sessionId) => {
        const state = get();
        const session = getSession(state.sessions, tabId);

        set({
          sessions: {
            ...state.sessions,
            [tabId]: { ...session, sessionId, lastActivity: Date.now() },
          },
        });
      },

      setStatus: (tabId, status, error = null) => {
        const state = get();
        const session = getSession(state.sessions, tabId);

        set({
          sessions: {
            ...state.sessions,
            [tabId]: { ...session, status, error, lastActivity: Date.now() },
          },
        });
      },

      setMessages: (tabId, messages) => {
        const state = get();
        const session = getSession(state.sessions, tabId);

        set({
          sessions: {
            ...state.sessions,
            [tabId]: { ...session, messages, lastActivity: Date.now() },
          },
        });
      },

      addMessage: (tabId, message) => {
        const state = get();
        const session = getSession(state.sessions, tabId);

        set({
          sessions: {
            ...state.sessions,
            [tabId]: {
              ...session,
              messages: [...session.messages, message],
              lastActivity: Date.now(),
            },
          },
        });
      },

      updateLastMessage: (tabId, updater) => {
        const state = get();
        const session = getSession(state.sessions, tabId);
        if (session.messages.length === 0) return;

        const messages = [...session.messages];
        const lastMessage = messages.at(-1);
        if (lastMessage) {
          messages[messages.length - 1] = updater(lastMessage);
        }

        set({
          sessions: {
            ...state.sessions,
            [tabId]: { ...session, messages, lastActivity: Date.now() },
          },
        });
      },

      clearMessages: (tabId) => {
        const state = get();
        const session = getSession(state.sessions, tabId);

        set({
          sessions: {
            ...state.sessions,
            [tabId]: {
              ...session,
              messages: [],
              sessionId: null,
              lastActivity: Date.now(),
            },
          },
        });
      },
    }),
    {
      name: STORAGE_KEY,
      // Only persist tabs, not session messages (they can be large)
      partialize: (state) => ({
        tabs: state.tabs,
        activeTabId: state.activeTabId,
      }),
    }
  )
);

// ============================================================
// Selector hooks for convenience
// ============================================================

// Stable default session to prevent infinite re-renders
const DEFAULT_SESSION: SessionState = {
  sessionId: null,
  status: "disconnected",
  messages: [],
  error: null,
  lastActivity: 0,
};

/**
 * Get session state for a specific tab.
 * Returns default state if session doesn't exist.
 */
export function useSession(tabId: string) {
  return useConsoleStore((state) => state.sessions[tabId] || DEFAULT_SESSION);
}

/**
 * Get just the tabs and activeTabId.
 */
export function useTabs() {
  return useConsoleStore((state) => ({
    tabs: state.tabs,
    activeTabId: state.activeTabId,
  }));
}

/**
 * Get the active tab.
 */
export function useActiveTab() {
  return useConsoleStore((state) =>
    state.tabs.find((t) => t.id === state.activeTabId) || null
  );
}

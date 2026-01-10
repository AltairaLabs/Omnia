"use client";

import { useCallback, useSyncExternalStore } from "react";
import type { ConsoleTab, ConsoleTabState, ConsoleTabStore } from "@/types/console-tabs";
import { clearConsoleState } from "./use-console-store";

const STORAGE_KEY = "omnia-console-tabs";
const MAX_TABS = 10;

/**
 * Module-level state for console tabs.
 * Persists across component unmounts and syncs with localStorage.
 */

let tabState: ConsoleTabState = {
  tabs: [],
  activeTabId: null,
};

const listeners = new Set<() => void>();

// Load initial state from localStorage
function loadFromStorage(): void {
  if (typeof window === "undefined") return;

  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored) as ConsoleTabState;
      // Validate the structure
      if (Array.isArray(parsed.tabs)) {
        tabState = {
          tabs: parsed.tabs.filter(
            (tab): tab is ConsoleTab =>
              typeof tab.id === "string" &&
              (tab.state === "selecting" || tab.state === "active") &&
              typeof tab.createdAt === "number"
          ),
          activeTabId: parsed.activeTabId,
        };
        // Ensure activeTabId is valid
        if (tabState.activeTabId && !tabState.tabs.find((t) => t.id === tabState.activeTabId)) {
          tabState.activeTabId = tabState.tabs[0]?.id ?? null;
        }
      }
    }
  } catch {
    // Invalid stored data, use default
  }
}

// Save state to localStorage
function saveToStorage(): void {
  if (typeof window === "undefined") return;

  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(tabState));
  } catch {
    // Storage full or unavailable
  }
}

function notifyListeners(): void {
  listeners.forEach((listener) => listener());
}

function generateTabId(): string {
  return `tab-${Date.now()}-${crypto.randomUUID().slice(0, 8)}`;
}

function subscribe(callback: () => void): () => void {
  listeners.add(callback);
  return () => listeners.delete(callback);
}

function getSnapshot(): ConsoleTabState {
  return tabState;
}

function getServerSnapshot(): ConsoleTabState {
  return { tabs: [], activeTabId: null };
}

// Initialize from storage on first load
let initialized = false;
function ensureInitialized(): void {
  if (!initialized && typeof window !== "undefined") {
    initialized = true;
    loadFromStorage();
  }
}

/**
 * Hook to manage console tab state.
 * Provides functions to create, close, and manage tabs.
 * State persists across page reloads via localStorage.
 */
export function useConsoleTabStore(): ConsoleTabStore {
  ensureInitialized();

  const state = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  const createTab = useCallback((): string => {
    // Enforce max tab limit
    if (tabState.tabs.length >= MAX_TABS) {
      // Remove oldest inactive tab if at limit
      const oldestInactive = tabState.tabs
        .filter((t) => t.id !== tabState.activeTabId)
        .sort((a, b) => a.createdAt - b.createdAt)[0];
      if (oldestInactive) {
        clearConsoleState(oldestInactive.id);
        tabState = {
          ...tabState,
          tabs: tabState.tabs.filter((t) => t.id !== oldestInactive.id),
        };
      }
    }

    const newTab: ConsoleTab = {
      id: generateTabId(),
      state: "selecting",
      createdAt: Date.now(),
    };

    tabState = {
      tabs: [...tabState.tabs, newTab],
      activeTabId: newTab.id,
    };

    saveToStorage();
    notifyListeners();
    return newTab.id;
  }, []);

  const closeTab = useCallback((id: string): void => {
    const tabIndex = tabState.tabs.findIndex((t) => t.id === id);
    if (tabIndex === -1) return;

    // Clear the console state for this tab
    clearConsoleState(id);

    const newTabs = tabState.tabs.filter((t) => t.id !== id);
    let newActiveId = tabState.activeTabId;

    // If we're closing the active tab, select an adjacent one
    if (tabState.activeTabId === id) {
      if (newTabs.length > 0) {
        // Prefer the tab to the right, then to the left
        const nextIndex = Math.min(tabIndex, newTabs.length - 1);
        newActiveId = newTabs[nextIndex].id;
      } else {
        newActiveId = null;
      }
    }

    tabState = {
      tabs: newTabs,
      activeTabId: newActiveId,
    };

    saveToStorage();
    notifyListeners();
  }, []);

  const setActiveTab = useCallback((id: string): void => {
    if (!tabState.tabs.find((t) => t.id === id)) return;
    if (tabState.activeTabId === id) return;

    tabState = {
      ...tabState,
      activeTabId: id,
    };

    saveToStorage();
    notifyListeners();
  }, []);

  const updateTab = useCallback((id: string, updates: Partial<Omit<ConsoleTab, "id">>): void => {
    const tabIndex = tabState.tabs.findIndex((t) => t.id === id);
    if (tabIndex === -1) return;

    const newTabs = [...tabState.tabs];
    newTabs[tabIndex] = {
      ...newTabs[tabIndex],
      ...updates,
    };

    tabState = {
      ...tabState,
      tabs: newTabs,
    };

    saveToStorage();
    notifyListeners();
  }, []);

  const clearAllTabs = useCallback((): void => {
    // Clear console state for all tabs
    tabState.tabs.forEach((tab) => {
      clearConsoleState(tab.id);
    });

    tabState = {
      tabs: [],
      activeTabId: null,
    };

    saveToStorage();
    notifyListeners();
  }, []);

  return {
    ...state,
    createTab,
    closeTab,
    setActiveTab,
    updateTab,
    clearAllTabs,
  };
}

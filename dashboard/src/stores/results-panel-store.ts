"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";

// =============================================================================
// Types
// =============================================================================

export type ResultsPanelTab = "problems" | "logs" | "results" | "console";

export interface ResultsPanelState {
  /** Whether the panel is open */
  isOpen: boolean;
  /** Currently active tab */
  activeTab: ResultsPanelTab;
  /** Panel height as percentage (0-100) */
  height: number;
  /** Current job being viewed (if any) */
  currentJobName: string | null;
  /** Problems count for badge */
  problemsCount: number;
}

export interface ResultsPanelActions {
  /** Open the panel */
  open: (tab?: ResultsPanelTab) => void;
  /** Close the panel */
  close: () => void;
  /** Toggle panel open/closed */
  toggle: () => void;
  /** Set the active tab */
  setActiveTab: (tab: ResultsPanelTab) => void;
  /** Set panel height */
  setHeight: (height: number) => void;
  /** Set current job */
  setCurrentJob: (jobName: string | null) => void;
  /** Set problems count */
  setProblemsCount: (count: number) => void;
  /** Open logs for a specific job */
  openJobLogs: (jobName: string) => void;
  /** Open results for a specific job */
  openJobResults: (jobName: string) => void;
}

export type ResultsPanelStore = ResultsPanelState & ResultsPanelActions;

// =============================================================================
// Constants
// =============================================================================

const DEFAULT_HEIGHT = 30; // 30% of editor height
const MIN_HEIGHT = 15;
const MAX_HEIGHT = 70;
const STORAGE_KEY = "omnia-results-panel";

// =============================================================================
// Store
// =============================================================================

export const useResultsPanelStore = create<ResultsPanelStore>()(
  persist(
    (set) => ({
      // Initial state
      isOpen: false,
      activeTab: "problems",
      height: DEFAULT_HEIGHT,
      currentJobName: null,
      problemsCount: 0,

      // Actions
      open: (tab) => {
        set((state) => ({
          isOpen: true,
          activeTab: tab ?? state.activeTab,
        }));
      },

      close: () => {
        set({ isOpen: false });
      },

      toggle: () => {
        set((state) => ({ isOpen: !state.isOpen }));
      },

      setActiveTab: (tab) => {
        set({ activeTab: tab });
      },

      setHeight: (height) => {
        // Clamp height to valid range
        const clampedHeight = Math.min(MAX_HEIGHT, Math.max(MIN_HEIGHT, height));
        set({ height: clampedHeight });
      },

      setCurrentJob: (jobName) => {
        set({ currentJobName: jobName });
      },

      setProblemsCount: (count) => {
        set({ problemsCount: count });
      },

      openJobLogs: (jobName) => {
        set({
          isOpen: true,
          activeTab: "logs",
          currentJobName: jobName,
        });
      },

      openJobResults: (jobName) => {
        set({
          isOpen: true,
          activeTab: "results",
          currentJobName: jobName,
        });
      },
    }),
    {
      name: STORAGE_KEY,
      // Only persist layout preferences, not runtime state
      partialize: (state) => ({
        height: state.height,
        activeTab: state.activeTab,
      }),
    }
  )
);

// =============================================================================
// Selector hooks for convenience
// =============================================================================

/**
 * Get the panel visibility state.
 */
export function useResultsPanelOpen(): boolean {
  return useResultsPanelStore((state) => state.isOpen);
}

/**
 * Get the active tab.
 */
export function useResultsPanelActiveTab(): ResultsPanelTab {
  return useResultsPanelStore((state) => state.activeTab);
}

/**
 * Get the current job name.
 */
export function useResultsPanelCurrentJob(): string | null {
  return useResultsPanelStore((state) => state.currentJobName);
}

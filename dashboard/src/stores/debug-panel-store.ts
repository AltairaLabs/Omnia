"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";

// =============================================================================
// Types
// =============================================================================

export type DebugPanelTab = "timeline" | "toolcalls" | "raw";

export interface DebugPanelState {
  /** Whether the panel is open */
  isOpen: boolean;
  /** Currently active tab */
  activeTab: DebugPanelTab;
  /** Panel height as percentage (15-70) */
  height: number;
  /** Currently selected tool call ID */
  selectedToolCallId: string | null;
}

export interface DebugPanelActions {
  /** Open the panel */
  open: (tab?: DebugPanelTab) => void;
  /** Close the panel */
  close: () => void;
  /** Toggle panel open/closed */
  toggle: () => void;
  /** Set the active tab */
  setActiveTab: (tab: DebugPanelTab) => void;
  /** Set panel height */
  setHeight: (height: number) => void;
  /** Select a tool call by ID */
  selectToolCall: (id: string | null) => void;
  /** Open the tool calls tab and select a specific tool call */
  openToolCall: (id: string) => void;
}

export type DebugPanelStore = DebugPanelState & DebugPanelActions;

// =============================================================================
// Constants
// =============================================================================

const DEFAULT_HEIGHT = 30;
const MIN_HEIGHT = 15;
const MAX_HEIGHT = 70;
const STORAGE_KEY = "omnia-debug-panel";

// =============================================================================
// Store
// =============================================================================

export const useDebugPanelStore = create<DebugPanelStore>()(
  persist(
    (set) => ({
      // Initial state
      isOpen: false,
      activeTab: "timeline",
      height: DEFAULT_HEIGHT,
      selectedToolCallId: null,

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
        const clampedHeight = Math.min(MAX_HEIGHT, Math.max(MIN_HEIGHT, height));
        set({ height: clampedHeight });
      },

      selectToolCall: (id) => {
        set({ selectedToolCallId: id });
      },

      openToolCall: (id) => {
        set({
          isOpen: true,
          activeTab: "toolcalls",
          selectedToolCallId: id,
        });
      },
    }),
    {
      name: STORAGE_KEY,
      partialize: (state) => ({
        height: state.height,
        activeTab: state.activeTab,
      }),
    }
  )
);

// =============================================================================
// Selector hooks
// =============================================================================

export function useDebugPanelOpen(): boolean {
  return useDebugPanelStore((state) => state.isOpen);
}

export function useDebugPanelActiveTab(): DebugPanelTab {
  return useDebugPanelStore((state) => state.activeTab);
}

export function useDebugPanelSelectedToolCall(): string | null {
  return useDebugPanelStore((state) => state.selectedToolCallId);
}

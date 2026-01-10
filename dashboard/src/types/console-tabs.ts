/**
 * Types for multi-session console tab management.
 */

export interface ConsoleTab {
  /** Unique identifier for the tab - also used as the console store key */
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

export interface ConsoleTabState {
  /** List of open tabs */
  tabs: ConsoleTab[];
  /** ID of the currently active tab */
  activeTabId: string | null;
}

export interface ConsoleTabStore extends ConsoleTabState {
  /** Create a new tab in selecting state */
  createTab: () => string;
  /** Close a tab by ID */
  closeTab: (id: string) => void;
  /** Set the active tab */
  setActiveTab: (id: string) => void;
  /** Update a tab's properties */
  updateTab: (id: string, updates: Partial<Omit<ConsoleTab, "id">>) => void;
  /** Clear all tabs and reset state */
  clearAllTabs: () => void;
}

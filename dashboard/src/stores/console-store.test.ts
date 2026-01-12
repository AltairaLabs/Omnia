import { describe, it, expect, beforeEach, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";

// Mock crypto.randomUUID with unique values
let uuidCounter = 0;
vi.stubGlobal("crypto", {
  randomUUID: () => `${++uuidCounter}-1234-1234-1234-123456789abc`,
});

// Need to import after mocking
import {
  useConsoleStore,
  useSession,
  useActiveTab,
} from "./console-store";
import type { ConsoleMessage, ConsoleMessageRole } from "@/types/websocket";

// Helper to create valid ConsoleMessage objects
let messageCounter = 0;
function createMessage(role: ConsoleMessageRole, content: string): ConsoleMessage {
  return {
    id: `msg-${Date.now()}-${++messageCounter}`,
    role,
    content,
    timestamp: new Date(),
  };
}

describe("useConsoleStore", () => {
  beforeEach(() => {
    uuidCounter = 0;
    // Reset store state before each test using setState directly
    useConsoleStore.setState({
      tabs: [],
      activeTabId: null,
      sessions: {},
    });
  });

  describe("tab management", () => {
    it("should create a new tab", () => {
      const store = useConsoleStore.getState();

      act(() => {
        store.createTab();
      });

      const state = useConsoleStore.getState();
      expect(state.tabs).toHaveLength(1);
      expect(state.tabs[0].state).toBe("selecting");
      expect(state.activeTabId).toBe(state.tabs[0].id);
    });

    it("should create multiple tabs", () => {
      const store = useConsoleStore.getState();

      act(() => {
        store.createTab();
        store.createTab();
        store.createTab();
      });

      const state = useConsoleStore.getState();
      expect(state.tabs).toHaveLength(3);
    });

    it("should close a tab", () => {
      const store = useConsoleStore.getState();
      let tabId = "";

      act(() => {
        tabId = store.createTab();
      });

      act(() => {
        store.closeTab(tabId);
      });

      const state = useConsoleStore.getState();
      expect(state.tabs).toHaveLength(0);
      expect(state.activeTabId).toBeNull();
    });

    it("should select adjacent tab when closing active tab", () => {
      const store = useConsoleStore.getState();
      let tabId1 = "";
      let tabId2 = "";

      act(() => {
        tabId1 = store.createTab();
        tabId2 = store.createTab();
      });

      // Close the active tab (tabId2)
      act(() => {
        store.closeTab(tabId2);
      });

      const state = useConsoleStore.getState();
      expect(state.tabs).toHaveLength(1);
      expect(state.activeTabId).toBe(tabId1);
    });

    it("should set active tab", () => {
      const store = useConsoleStore.getState();
      let tabId1 = "";

      act(() => {
        tabId1 = store.createTab();
        store.createTab(); // Create second tab to have something to switch from
      });

      act(() => {
        store.setActiveTab(tabId1);
      });

      const state = useConsoleStore.getState();
      expect(state.activeTabId).toBe(tabId1);
    });

    it("should not set active tab for non-existent tab", () => {
      const store = useConsoleStore.getState();
      let tabId = "";

      act(() => {
        tabId = store.createTab();
      });

      act(() => {
        store.setActiveTab("non-existent");
      });

      const state = useConsoleStore.getState();
      expect(state.activeTabId).toBe(tabId);
    });

    it("should not change active tab if already active", () => {
      const store = useConsoleStore.getState();
      let tabId = "";

      act(() => {
        tabId = store.createTab();
      });

      const initialState = useConsoleStore.getState();

      act(() => {
        store.setActiveTab(tabId);
      });

      // State should be identical (no unnecessary re-renders)
      expect(useConsoleStore.getState().activeTabId).toBe(initialState.activeTabId);
    });

    it("should update tab properties", () => {
      const store = useConsoleStore.getState();
      let tabId = "";

      act(() => {
        tabId = store.createTab();
      });

      act(() => {
        store.updateTab(tabId, {
          state: "active",
          agentName: "test-agent",
          namespace: "production",
        });
      });

      const state = useConsoleStore.getState();
      const tab = state.tabs.find((t) => t.id === tabId);
      expect(tab?.state).toBe("active");
      expect(tab?.agentName).toBe("test-agent");
      expect(tab?.namespace).toBe("production");
    });

    it("should not update non-existent tab", () => {
      const store = useConsoleStore.getState();

      act(() => {
        store.createTab();
      });

      const beforeState = useConsoleStore.getState();

      act(() => {
        store.updateTab("non-existent", { state: "active" });
      });

      expect(useConsoleStore.getState().tabs).toEqual(beforeState.tabs);
    });

    it("should clear all tabs", () => {
      const store = useConsoleStore.getState();

      act(() => {
        store.createTab();
        store.createTab();
      });

      act(() => {
        store.clearAllTabs();
      });

      const state = useConsoleStore.getState();
      expect(state.tabs).toHaveLength(0);
      expect(state.activeTabId).toBeNull();
      expect(state.sessions).toEqual({});
    });

    it("should not close non-existent tab", () => {
      const store = useConsoleStore.getState();

      act(() => {
        store.createTab();
      });

      const beforeState = useConsoleStore.getState();

      act(() => {
        store.closeTab("non-existent");
      });

      expect(useConsoleStore.getState().tabs).toEqual(beforeState.tabs);
    });

    it("should enforce max tab limit by removing oldest inactive tab", () => {
      const store = useConsoleStore.getState();
      const tabIds: string[] = [];

      // Create 10 tabs (max limit)
      act(() => {
        for (let i = 0; i < 10; i++) {
          tabIds.push(store.createTab());
        }
      });

      expect(useConsoleStore.getState().tabs).toHaveLength(10);

      // Switch to a different tab so there are inactive tabs
      act(() => {
        store.setActiveTab(tabIds[5]);
      });

      // Create one more - should remove oldest inactive (tabIds[0])
      act(() => {
        store.createTab();
      });

      const state = useConsoleStore.getState();
      expect(state.tabs).toHaveLength(10);
      // First tab should have been removed
      expect(state.tabs.find((t) => t.id === tabIds[0])).toBeUndefined();
    });
  });

  describe("session management", () => {
    it("should set session ID", () => {
      const store = useConsoleStore.getState();
      let tabId = "";

      act(() => {
        tabId = store.createTab();
      });

      act(() => {
        store.setSessionId(tabId, "session-123");
      });

      const state = useConsoleStore.getState();
      expect(state.sessions[tabId]?.sessionId).toBe("session-123");
    });

    it("should set connection status", () => {
      const store = useConsoleStore.getState();
      let tabId = "";

      act(() => {
        tabId = store.createTab();
      });

      act(() => {
        store.setStatus(tabId, "connected");
      });

      const state = useConsoleStore.getState();
      expect(state.sessions[tabId]?.status).toBe("connected");
    });

    it("should set connection status with error", () => {
      const store = useConsoleStore.getState();
      let tabId = "";

      act(() => {
        tabId = store.createTab();
      });

      act(() => {
        store.setStatus(tabId, "error", "Connection failed");
      });

      const state = useConsoleStore.getState();
      expect(state.sessions[tabId]?.status).toBe("error");
      expect(state.sessions[tabId]?.error).toBe("Connection failed");
    });

    it("should set messages", () => {
      const store = useConsoleStore.getState();
      let tabId = "";

      act(() => {
        tabId = store.createTab();
      });

      const messages: ConsoleMessage[] = [
        createMessage("user", "Hello"),
        createMessage("assistant", "Hi there!"),
      ];

      act(() => {
        store.setMessages(tabId, messages);
      });

      const state = useConsoleStore.getState();
      expect(state.sessions[tabId]?.messages).toHaveLength(2);
      expect(state.sessions[tabId]?.messages[0].content).toBe("Hello");
      expect(state.sessions[tabId]?.messages[1].content).toBe("Hi there!");
    });

    it("should add a message", () => {
      const store = useConsoleStore.getState();
      let tabId = "";

      act(() => {
        tabId = store.createTab();
      });

      act(() => {
        store.addMessage(tabId, createMessage("user", "Hello"));
      });

      const state = useConsoleStore.getState();
      expect(state.sessions[tabId]?.messages).toHaveLength(1);
      expect(state.sessions[tabId]?.messages[0].content).toBe("Hello");
    });

    it("should update last message", () => {
      const store = useConsoleStore.getState();
      let tabId = "";

      act(() => {
        tabId = store.createTab();
      });

      act(() => {
        store.addMessage(tabId, createMessage("assistant", "Hello"));
      });

      act(() => {
        store.updateLastMessage(tabId, (msg) => ({
          ...msg,
          content: "Hello, updated!",
        }));
      });

      const state = useConsoleStore.getState();
      expect(state.sessions[tabId]?.messages[0].content).toBe("Hello, updated!");
    });

    it("should not update last message when no messages exist", () => {
      const store = useConsoleStore.getState();
      let tabId = "";

      act(() => {
        tabId = store.createTab();
      });

      // Should not throw
      act(() => {
        store.updateLastMessage(tabId, (msg) => ({
          ...msg,
          content: "Updated",
        }));
      });

      const state = useConsoleStore.getState();
      expect(state.sessions[tabId]?.messages || []).toHaveLength(0);
    });

    it("should clear messages", () => {
      const store = useConsoleStore.getState();
      let tabId = "";

      act(() => {
        tabId = store.createTab();
      });

      act(() => {
        store.addMessage(tabId, createMessage("user", "Hello"));
        store.setSessionId(tabId, "session-123");
      });

      act(() => {
        store.clearMessages(tabId);
      });

      const state = useConsoleStore.getState();
      expect(state.sessions[tabId]?.messages).toHaveLength(0);
      expect(state.sessions[tabId]?.sessionId).toBeNull();
    });
  });

  describe("selector hooks", () => {
    it("useSession should return default session for non-existent tab", () => {
      const { result } = renderHook(() => useSession("non-existent"));

      expect(result.current.sessionId).toBeNull();
      expect(result.current.status).toBe("disconnected");
      expect(result.current.messages).toEqual([]);
    });

    it("useSession should return session for existing tab", () => {
      const store = useConsoleStore.getState();
      let tabId = "";

      act(() => {
        tabId = store.createTab();
        store.setSessionId(tabId, "session-456");
      });

      const { result } = renderHook(() => useSession(tabId));

      expect(result.current.sessionId).toBe("session-456");
    });

    it("should access tabs directly from store", () => {
      const store = useConsoleStore.getState();

      act(() => {
        store.createTab();
      });

      const state = useConsoleStore.getState();
      expect(state.tabs).toHaveLength(1);
      expect(state.activeTabId).toBe(state.tabs[0].id);
    });

    it("useActiveTab should return null when no tabs", () => {
      const { result } = renderHook(() => useActiveTab());

      expect(result.current).toBeNull();
    });

    it("useActiveTab should return active tab", () => {
      const store = useConsoleStore.getState();

      act(() => {
        store.createTab();
      });

      const { result } = renderHook(() => useActiveTab());

      expect(result.current).not.toBeNull();
      expect(result.current?.state).toBe("selecting");
    });
  });
});

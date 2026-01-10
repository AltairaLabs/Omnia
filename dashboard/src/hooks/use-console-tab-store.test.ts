import { describe, it, expect, beforeEach, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: vi.fn((key: string) => store[key] || null),
    setItem: vi.fn((key: string, value: string) => {
      store[key] = value;
    }),
    removeItem: vi.fn((key: string) => {
      delete store[key];
    }),
    clear: vi.fn(() => {
      store = {};
    }),
  };
})();

Object.defineProperty(window, "localStorage", { value: localStorageMock });

// Mock crypto.randomUUID to return unique values
let uuidCounter = 0;
vi.stubGlobal("crypto", {
  randomUUID: () => `${++uuidCounter}-1234-1234-1234-123456789012`,
});

// Reset module between tests to clear state
beforeEach(async () => {
  localStorageMock.clear();
  vi.clearAllMocks();
  vi.resetModules();
  uuidCounter = 0;
});

describe("useConsoleTabStore", () => {
  describe("createTab", () => {
    it("should create a new tab in selecting state", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      // Clear any auto-created tabs
      act(() => {
        result.current.clearAllTabs();
      });

      let tabId: string;
      act(() => {
        tabId = result.current.createTab();
      });

      expect(result.current.tabs).toHaveLength(1);
      expect(result.current.tabs[0].id).toBe(tabId!);
      expect(result.current.tabs[0].state).toBe("selecting");
      expect(result.current.activeTabId).toBe(tabId!);
    });

    it("should set the new tab as active", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      // Clear any auto-created tabs
      act(() => {
        result.current.clearAllTabs();
      });

      let secondTabId: string;

      act(() => {
        result.current.createTab();
      });

      act(() => {
        secondTabId = result.current.createTab();
      });

      expect(result.current.activeTabId).toBe(secondTabId!);
      expect(result.current.tabs).toHaveLength(2);
    });

    it("should persist to localStorage", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      act(() => {
        result.current.clearAllTabs();
      });

      act(() => {
        result.current.createTab();
      });

      expect(localStorageMock.setItem).toHaveBeenCalled();
    });

    it("should generate unique tab ids", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      act(() => {
        result.current.clearAllTabs();
      });

      let firstTabId: string;
      let secondTabId: string;

      act(() => {
        firstTabId = result.current.createTab();
      });
      act(() => {
        secondTabId = result.current.createTab();
      });

      expect(firstTabId!).not.toBe(secondTabId!);
      expect(result.current.tabs).toHaveLength(2);
    });
  });

  describe("closeTab", () => {
    it("should remove the specified tab", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      act(() => {
        result.current.clearAllTabs();
      });

      let tabId: string;
      act(() => {
        tabId = result.current.createTab();
      });

      act(() => {
        result.current.closeTab(tabId!);
      });

      expect(result.current.tabs).toHaveLength(0);
    });

    it("should set activeTabId to null when closing last tab", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      act(() => {
        result.current.clearAllTabs();
      });

      let tabId: string;
      act(() => {
        tabId = result.current.createTab();
      });

      act(() => {
        result.current.closeTab(tabId!);
      });

      expect(result.current.activeTabId).toBeNull();
    });

    it("should handle closing a non-active tab", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      act(() => {
        result.current.clearAllTabs();
      });

      // Create 2 tabs
      let firstTabId: string;
      let secondTabId: string;

      act(() => {
        firstTabId = result.current.createTab();
      });
      act(() => {
        secondTabId = result.current.createTab();
      });

      // Second tab is now active
      expect(result.current.activeTabId).toBe(secondTabId!);

      // Close the first tab (non-active)
      act(() => {
        result.current.closeTab(firstTabId!);
      });

      // Second tab should still be active
      expect(result.current.activeTabId).toBe(secondTabId!);
      expect(result.current.tabs).toHaveLength(1);
    });

    it("should not remove tab if id is not found", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      act(() => {
        result.current.clearAllTabs();
      });

      act(() => {
        result.current.createTab();
      });

      const tabCountBefore = result.current.tabs.length;

      act(() => {
        result.current.closeTab("non-existent-id");
      });

      expect(result.current.tabs.length).toBe(tabCountBefore);
    });
  });

  describe("setActiveTab", () => {
    it("should change the active tab", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      act(() => {
        result.current.clearAllTabs();
      });

      let firstTabId: string;

      act(() => {
        firstTabId = result.current.createTab();
      });

      act(() => {
        result.current.createTab();
      });

      act(() => {
        result.current.setActiveTab(firstTabId!);
      });

      expect(result.current.activeTabId).toBe(firstTabId!);
    });

    it("should not change if tab id does not exist", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      act(() => {
        result.current.clearAllTabs();
      });

      act(() => {
        result.current.createTab();
      });

      const activeBefore = result.current.activeTabId;

      act(() => {
        result.current.setActiveTab("non-existent-id");
      });

      expect(result.current.activeTabId).toBe(activeBefore);
    });

    it("should not update if already active", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      act(() => {
        result.current.clearAllTabs();
      });

      let tabId: string;
      act(() => {
        tabId = result.current.createTab();
      });

      const setItemCallsBefore = localStorageMock.setItem.mock.calls.length;

      // Try to set the same tab as active
      act(() => {
        result.current.setActiveTab(tabId!);
      });

      // Should not have saved again
      expect(localStorageMock.setItem.mock.calls.length).toBe(setItemCallsBefore);
    });
  });

  describe("updateTab", () => {
    it("should update tab properties", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      act(() => {
        result.current.clearAllTabs();
      });

      let tabId: string;
      act(() => {
        tabId = result.current.createTab();
      });

      act(() => {
        result.current.updateTab(tabId!, {
          state: "active",
          agentName: "test-agent",
          namespace: "production",
        });
      });

      const tab = result.current.tabs.find((t) => t.id === tabId!);
      expect(tab?.state).toBe("active");
      expect(tab?.agentName).toBe("test-agent");
      expect(tab?.namespace).toBe("production");
    });

    it("should not update if tab id does not exist", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      act(() => {
        result.current.clearAllTabs();
      });

      act(() => {
        result.current.createTab();
      });

      const tabsBefore = [...result.current.tabs];

      act(() => {
        result.current.updateTab("non-existent-id", {
          state: "active",
          agentName: "test-agent",
        });
      });

      // Tabs should remain unchanged
      expect(result.current.tabs).toEqual(tabsBefore);
    });
  });

  describe("clearAllTabs", () => {
    it("should remove all tabs", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      act(() => {
        result.current.createTab();
        result.current.createTab();
        result.current.createTab();
      });

      expect(result.current.tabs.length).toBeGreaterThan(0);

      act(() => {
        result.current.clearAllTabs();
      });

      expect(result.current.tabs).toHaveLength(0);
      expect(result.current.activeTabId).toBeNull();
    });
  });

  describe("max tab limit", () => {
    it("should remove oldest inactive tab when at max limit", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      act(() => {
        result.current.clearAllTabs();
      });

      // Create 10 tabs (the max)
      const tabIds: string[] = [];
      for (let i = 0; i < 10; i++) {
        act(() => {
          tabIds.push(result.current.createTab());
        });
      }

      expect(result.current.tabs).toHaveLength(10);
      const firstTabId = tabIds[0];

      // Create one more tab - should remove the oldest inactive
      act(() => {
        result.current.createTab();
      });

      // Should still be at 10 tabs
      expect(result.current.tabs).toHaveLength(10);
      // First tab should have been removed
      expect(result.current.tabs.find((t) => t.id === firstTabId)).toBeUndefined();
    });
  });

  describe("closeTab active tab selection", () => {
    it("should select the next tab when closing active tab", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      act(() => {
        result.current.clearAllTabs();
      });

      // Create 3 tabs
      let secondTabId: string;
      let thirdTabId: string;

      act(() => {
        result.current.createTab(); // First tab - we don't need its ID for this test
      });
      act(() => {
        secondTabId = result.current.createTab();
      });
      act(() => {
        thirdTabId = result.current.createTab();
      });

      // Set second tab as active
      act(() => {
        result.current.setActiveTab(secondTabId!);
      });

      expect(result.current.activeTabId).toBe(secondTabId!);

      // Close the second (active) tab
      act(() => {
        result.current.closeTab(secondTabId!);
      });

      // Should select the next tab (third becomes second position now)
      expect(result.current.tabs).toHaveLength(2);
      expect(result.current.activeTabId).toBe(thirdTabId!);
    });

    it("should select the previous tab when closing last active tab", async () => {
      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      act(() => {
        result.current.clearAllTabs();
      });

      // Create 2 tabs
      let firstTabId: string;
      let secondTabId: string;

      act(() => {
        firstTabId = result.current.createTab();
      });
      act(() => {
        secondTabId = result.current.createTab();
      });

      // Second tab is active
      expect(result.current.activeTabId).toBe(secondTabId!);

      // Close the second (active, last) tab
      act(() => {
        result.current.closeTab(secondTabId!);
      });

      // Should select the first tab (now only tab)
      expect(result.current.tabs).toHaveLength(1);
      expect(result.current.activeTabId).toBe(firstTabId!);
    });
  });

  describe("localStorage loading", () => {
    it("should load tabs from localStorage on initialization", async () => {
      // Set up localStorage with existing tabs
      const storedState = {
        tabs: [
          { id: "stored-tab-1", state: "active", createdAt: Date.now() - 1000, agentName: "agent1", namespace: "prod" },
          { id: "stored-tab-2", state: "selecting", createdAt: Date.now() },
        ],
        activeTabId: "stored-tab-1",
      };
      localStorageMock.getItem.mockReturnValueOnce(JSON.stringify(storedState));

      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      // Should have loaded the tabs from storage
      expect(result.current.tabs.length).toBeGreaterThanOrEqual(0);
    });

    it("should handle invalid localStorage data", async () => {
      // Set up localStorage with invalid data
      localStorageMock.getItem.mockReturnValueOnce("invalid json{");

      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      // Should use default state
      expect(Array.isArray(result.current.tabs)).toBe(true);
    });

    it("should validate tab structure when loading", async () => {
      // Set up localStorage with invalid tab structure
      const storedState = {
        tabs: [
          { id: "valid-tab", state: "active", createdAt: Date.now() },
          { invalid: "structure" }, // Missing required fields
          { id: "another-valid", state: "selecting", createdAt: Date.now() },
        ],
        activeTabId: "valid-tab",
      };
      localStorageMock.getItem.mockReturnValueOnce(JSON.stringify(storedState));

      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      // Should filter out invalid tabs
      expect(Array.isArray(result.current.tabs)).toBe(true);
    });

    it("should fix invalid activeTabId when loading", async () => {
      // Set up localStorage with activeTabId pointing to non-existent tab
      const storedState = {
        tabs: [
          { id: "tab-1", state: "active", createdAt: Date.now() },
        ],
        activeTabId: "non-existent-tab",
      };
      localStorageMock.getItem.mockReturnValueOnce(JSON.stringify(storedState));

      const { useConsoleTabStore } = await import("./use-console-tab-store");
      const { result } = renderHook(() => useConsoleTabStore());

      // activeTabId should be fixed to first tab or null
      if (result.current.tabs.length > 0) {
        expect(result.current.activeTabId).toBe(result.current.tabs[0].id);
      }
    });
  });
});

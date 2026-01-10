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

// Mock crypto.randomUUID
vi.stubGlobal("crypto", {
  randomUUID: () => "12345678-1234-1234-1234-123456789012",
});

// Reset module between tests to clear state
beforeEach(async () => {
  localStorageMock.clear();
  vi.clearAllMocks();
  vi.resetModules();
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
});

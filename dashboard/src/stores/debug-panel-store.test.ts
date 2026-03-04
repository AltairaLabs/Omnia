import { describe, it, expect, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import {
  useDebugPanelStore,
  useDebugPanelOpen,
  useDebugPanelActiveTab,
  useDebugPanelSelectedToolCall,
  type DebugPanelTab,
} from "./debug-panel-store";

describe("debug-panel-store", () => {
  beforeEach(() => {
    useDebugPanelStore.setState({
      isOpen: false,
      activeTab: "timeline",
      height: 30,
      selectedToolCallId: null,
    });
  });

  describe("initial state", () => {
    it("should have correct initial values", () => {
      const state = useDebugPanelStore.getState();
      expect(state.isOpen).toBe(false);
      expect(state.activeTab).toBe("timeline");
      expect(state.height).toBe(30);
      expect(state.selectedToolCallId).toBe(null);
    });
  });

  describe("open", () => {
    it("should open the panel with the specified tab", () => {
      useDebugPanelStore.getState().open("toolcalls");
      const state = useDebugPanelStore.getState();
      expect(state.isOpen).toBe(true);
      expect(state.activeTab).toBe("toolcalls");
    });

    it("should open with current tab if none specified", () => {
      useDebugPanelStore.setState({ activeTab: "raw" });
      useDebugPanelStore.getState().open();
      const state = useDebugPanelStore.getState();
      expect(state.isOpen).toBe(true);
      expect(state.activeTab).toBe("raw");
    });
  });

  describe("close", () => {
    it("should close the panel", () => {
      useDebugPanelStore.setState({ isOpen: true });
      useDebugPanelStore.getState().close();
      expect(useDebugPanelStore.getState().isOpen).toBe(false);
    });
  });

  describe("toggle", () => {
    it("should toggle panel from closed to open", () => {
      useDebugPanelStore.setState({ isOpen: false });
      useDebugPanelStore.getState().toggle();
      expect(useDebugPanelStore.getState().isOpen).toBe(true);
    });

    it("should toggle panel from open to closed", () => {
      useDebugPanelStore.setState({ isOpen: true });
      useDebugPanelStore.getState().toggle();
      expect(useDebugPanelStore.getState().isOpen).toBe(false);
    });
  });

  describe("setActiveTab", () => {
    it("should set the active tab", () => {
      useDebugPanelStore.getState().setActiveTab("raw");
      expect(useDebugPanelStore.getState().activeTab).toBe("raw");
    });

    it("should accept all valid tab values", () => {
      const tabs: DebugPanelTab[] = ["timeline", "toolcalls", "raw"];
      for (const tab of tabs) {
        useDebugPanelStore.getState().setActiveTab(tab);
        expect(useDebugPanelStore.getState().activeTab).toBe(tab);
      }
    });
  });

  describe("setHeight", () => {
    it("should set height within valid range", () => {
      useDebugPanelStore.getState().setHeight(50);
      expect(useDebugPanelStore.getState().height).toBe(50);
    });

    it("should clamp height to minimum of 15%", () => {
      useDebugPanelStore.getState().setHeight(5);
      expect(useDebugPanelStore.getState().height).toBe(15);
    });

    it("should clamp height to maximum of 70%", () => {
      useDebugPanelStore.getState().setHeight(80);
      expect(useDebugPanelStore.getState().height).toBe(70);
    });
  });

  describe("selectToolCall", () => {
    it("should set the selected tool call ID", () => {
      useDebugPanelStore.getState().selectToolCall("tc-123");
      expect(useDebugPanelStore.getState().selectedToolCallId).toBe("tc-123");
    });

    it("should clear the selection when set to null", () => {
      useDebugPanelStore.setState({ selectedToolCallId: "tc-123" });
      useDebugPanelStore.getState().selectToolCall(null);
      expect(useDebugPanelStore.getState().selectedToolCallId).toBe(null);
    });
  });

  describe("openToolCall", () => {
    it("should open panel with toolcalls tab and select the tool call", () => {
      useDebugPanelStore.getState().openToolCall("tc-456");
      const state = useDebugPanelStore.getState();
      expect(state.isOpen).toBe(true);
      expect(state.activeTab).toBe("toolcalls");
      expect(state.selectedToolCallId).toBe("tc-456");
    });
  });

  describe("selector hooks", () => {
    it("useDebugPanelOpen should return isOpen state", () => {
      useDebugPanelStore.setState({ isOpen: true });
      const { result } = renderHook(() => useDebugPanelOpen());
      expect(result.current).toBe(true);

      useDebugPanelStore.setState({ isOpen: false });
      const { result: result2 } = renderHook(() => useDebugPanelOpen());
      expect(result2.current).toBe(false);
    });

    it("useDebugPanelActiveTab should return activeTab state", () => {
      useDebugPanelStore.setState({ activeTab: "raw" });
      const { result } = renderHook(() => useDebugPanelActiveTab());
      expect(result.current).toBe("raw");
    });

    it("useDebugPanelSelectedToolCall should return selectedToolCallId state", () => {
      useDebugPanelStore.setState({ selectedToolCallId: "tc-789" });
      const { result } = renderHook(() => useDebugPanelSelectedToolCall());
      expect(result.current).toBe("tc-789");
    });

    it("useDebugPanelSelectedToolCall should return null when no selection", () => {
      useDebugPanelStore.setState({ selectedToolCallId: null });
      const { result } = renderHook(() => useDebugPanelSelectedToolCall());
      expect(result.current).toBeNull();
    });
  });
});

import { describe, it, expect, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import {
  useResultsPanelStore,
  useResultsPanelOpen,
  useResultsPanelActiveTab,
  useResultsPanelCurrentJob,
  type ResultsPanelTab,
} from "./results-panel-store";

describe("results-panel-store", () => {
  beforeEach(() => {
    // Reset store state before each test
    useResultsPanelStore.setState({
      isOpen: false,
      activeTab: "problems",
      height: 30,
      currentJobName: null,
      problemsCount: 0,
    });
  });

  describe("initial state", () => {
    it("should have correct initial values", () => {
      const state = useResultsPanelStore.getState();
      expect(state.isOpen).toBe(false);
      expect(state.activeTab).toBe("problems");
      expect(state.height).toBe(30);
      expect(state.currentJobName).toBe(null);
      expect(state.problemsCount).toBe(0);
    });
  });

  describe("open", () => {
    it("should open the panel with the specified tab", () => {
      useResultsPanelStore.getState().open("logs");
      const state = useResultsPanelStore.getState();
      expect(state.isOpen).toBe(true);
      expect(state.activeTab).toBe("logs");
    });

    it("should open with current tab if none specified", () => {
      useResultsPanelStore.setState({ activeTab: "results" });
      useResultsPanelStore.getState().open();
      const state = useResultsPanelStore.getState();
      expect(state.isOpen).toBe(true);
      expect(state.activeTab).toBe("results");
    });
  });

  describe("close", () => {
    it("should close the panel", () => {
      useResultsPanelStore.setState({ isOpen: true });
      useResultsPanelStore.getState().close();
      expect(useResultsPanelStore.getState().isOpen).toBe(false);
    });
  });

  describe("toggle", () => {
    it("should toggle panel from closed to open", () => {
      useResultsPanelStore.setState({ isOpen: false });
      useResultsPanelStore.getState().toggle();
      expect(useResultsPanelStore.getState().isOpen).toBe(true);
    });

    it("should toggle panel from open to closed", () => {
      useResultsPanelStore.setState({ isOpen: true });
      useResultsPanelStore.getState().toggle();
      expect(useResultsPanelStore.getState().isOpen).toBe(false);
    });
  });

  describe("setActiveTab", () => {
    it("should set the active tab", () => {
      useResultsPanelStore.getState().setActiveTab("console");
      expect(useResultsPanelStore.getState().activeTab).toBe("console");
    });

    it("should accept all valid tab values", () => {
      const tabs: ResultsPanelTab[] = ["problems", "logs", "results", "console"];
      for (const tab of tabs) {
        useResultsPanelStore.getState().setActiveTab(tab);
        expect(useResultsPanelStore.getState().activeTab).toBe(tab);
      }
    });
  });

  describe("setHeight", () => {
    it("should set height within valid range", () => {
      useResultsPanelStore.getState().setHeight(50);
      expect(useResultsPanelStore.getState().height).toBe(50);
    });

    it("should clamp height to minimum of 15%", () => {
      useResultsPanelStore.getState().setHeight(5);
      expect(useResultsPanelStore.getState().height).toBe(15);
    });

    it("should clamp height to maximum of 70%", () => {
      useResultsPanelStore.getState().setHeight(80);
      expect(useResultsPanelStore.getState().height).toBe(70);
    });
  });

  describe("setCurrentJob", () => {
    it("should set the current job name", () => {
      useResultsPanelStore.getState().setCurrentJob("test-job-123");
      expect(useResultsPanelStore.getState().currentJobName).toBe("test-job-123");
    });

    it("should clear the current job name when set to null", () => {
      useResultsPanelStore.setState({ currentJobName: "some-job" });
      useResultsPanelStore.getState().setCurrentJob(null);
      expect(useResultsPanelStore.getState().currentJobName).toBe(null);
    });
  });

  describe("setProblemsCount", () => {
    it("should set the problems count", () => {
      useResultsPanelStore.getState().setProblemsCount(5);
      expect(useResultsPanelStore.getState().problemsCount).toBe(5);
    });
  });

  describe("openJobLogs", () => {
    it("should open panel with logs tab and set job name", () => {
      useResultsPanelStore.getState().openJobLogs("test-job-123");
      const state = useResultsPanelStore.getState();
      expect(state.isOpen).toBe(true);
      expect(state.activeTab).toBe("logs");
      expect(state.currentJobName).toBe("test-job-123");
    });
  });

  describe("openJobResults", () => {
    it("should open panel with results tab and set job name", () => {
      useResultsPanelStore.getState().openJobResults("test-job-456");
      const state = useResultsPanelStore.getState();
      expect(state.isOpen).toBe(true);
      expect(state.activeTab).toBe("results");
      expect(state.currentJobName).toBe("test-job-456");
    });
  });

  describe("selector hooks", () => {
    it("useResultsPanelOpen should return isOpen state", () => {
      useResultsPanelStore.setState({ isOpen: true });
      const { result } = renderHook(() => useResultsPanelOpen());
      expect(result.current).toBe(true);

      useResultsPanelStore.setState({ isOpen: false });
      const { result: result2 } = renderHook(() => useResultsPanelOpen());
      expect(result2.current).toBe(false);
    });

    it("useResultsPanelActiveTab should return activeTab state", () => {
      useResultsPanelStore.setState({ activeTab: "logs" });
      const { result } = renderHook(() => useResultsPanelActiveTab());
      expect(result.current).toBe("logs");
    });

    it("useResultsPanelCurrentJob should return currentJobName state", () => {
      useResultsPanelStore.setState({ currentJobName: "my-job" });
      const { result } = renderHook(() => useResultsPanelCurrentJob());
      expect(result.current).toBe("my-job");
    });

    it("useResultsPanelCurrentJob should return null when no job", () => {
      useResultsPanelStore.setState({ currentJobName: null });
      const { result } = renderHook(() => useResultsPanelCurrentJob());
      expect(result.current).toBeNull();
    });
  });
});

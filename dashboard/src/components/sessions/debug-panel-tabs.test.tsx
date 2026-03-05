import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { DebugPanelTabs } from "./debug-panel-tabs";
import { useDebugPanelStore } from "@/stores/debug-panel-store";

describe("DebugPanelTabs", () => {
  beforeEach(() => {
    useDebugPanelStore.setState({
      isOpen: true,
      activeTab: "timeline",
      height: 30,
      selectedToolCallId: null,
    });
  });

  it("renders all three tabs", () => {
    render(<DebugPanelTabs />);
    expect(screen.getByText("Timeline")).toBeInTheDocument();
    expect(screen.getByText("Tool Calls")).toBeInTheDocument();
    expect(screen.getByText("Raw")).toBeInTheDocument();
  });

  it("highlights the active tab", () => {
    useDebugPanelStore.setState({ activeTab: "raw" });
    render(<DebugPanelTabs />);

    const rawTab = screen.getByTestId("debug-tab-raw");
    expect(rawTab.className).toContain("border-primary");
  });

  it("switches tab on click", () => {
    render(<DebugPanelTabs />);
    fireEvent.click(screen.getByTestId("debug-tab-toolcalls"));
    expect(useDebugPanelStore.getState().activeTab).toBe("toolcalls");
  });

  it("shows tool call count badge when provided", () => {
    render(<DebugPanelTabs toolCallCount={5} />);
    expect(screen.getByText("5")).toBeInTheDocument();
  });

  it("does not show badge when count is 0", () => {
    render(<DebugPanelTabs toolCallCount={0} />);
    expect(screen.queryByText("0")).not.toBeInTheDocument();
  });

  it("does not show badge when count is undefined", () => {
    render(<DebugPanelTabs />);
    // No count badge should be visible
    const tabsContainer = screen.getByTestId("debug-panel-tabs");
    const badges = tabsContainer.querySelectorAll(".rounded-full");
    expect(badges).toHaveLength(0);
  });
});

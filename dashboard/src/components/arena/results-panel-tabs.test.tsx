import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ResultsPanelTabs } from "./results-panel-tabs";

// Mock state values
let mockActiveTab = "problems";
let mockProblemsCount = 0;
const mockSetActiveTab = vi.fn();

// Mock the results panel store
vi.mock("@/stores/results-panel-store", () => ({
  useResultsPanelStore: vi.fn((selector) => {
    const state = {
      activeTab: mockActiveTab,
      setActiveTab: mockSetActiveTab,
      problemsCount: mockProblemsCount,
    };
    return selector(state);
  }),
}));

describe("ResultsPanelTabs", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockActiveTab = "problems";
    mockProblemsCount = 0;
  });

  it("should render all tabs", () => {
    render(<ResultsPanelTabs />);

    expect(screen.getByText("Problems")).toBeInTheDocument();
    expect(screen.getByText("Logs")).toBeInTheDocument();
    expect(screen.getByText("Results")).toBeInTheDocument();
    expect(screen.getByText("Console")).toBeInTheDocument();
  });

  it("should highlight active tab", () => {
    mockActiveTab = "logs";
    render(<ResultsPanelTabs />);

    const logsTab = screen.getByText("Logs").closest("button");
    expect(logsTab).toHaveClass("bg-background");
  });

  it("should call setActiveTab when tab is clicked", async () => {
    const user = userEvent.setup();
    render(<ResultsPanelTabs />);

    const logsTab = screen.getByText("Logs").closest("button");
    await user.click(logsTab!);

    expect(mockSetActiveTab).toHaveBeenCalledWith("logs");
  });

  it("should show problems count badge when count > 0", () => {
    mockProblemsCount = 5;
    render(<ResultsPanelTabs />);

    expect(screen.getByText("5")).toBeInTheDocument();
  });

  it("should not show problems count badge when count is 0", () => {
    mockProblemsCount = 0;
    render(<ResultsPanelTabs />);

    // The problems label should exist but no badge number
    expect(screen.getByText("Problems")).toBeInTheDocument();
    expect(screen.queryByText("0")).not.toBeInTheDocument();
  });

  it("should apply custom className", () => {
    const { container } = render(<ResultsPanelTabs className="custom-class" />);
    expect(container.firstChild).toHaveClass("custom-class");
  });

  it("should render tab icons", () => {
    render(<ResultsPanelTabs />);

    // Each tab button should have an svg icon
    const buttons = screen.getAllByRole("button");
    buttons.forEach((button) => {
      expect(button.querySelector("svg")).toBeInTheDocument();
    });
  });
});

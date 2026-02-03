import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ResultsPanel, type Problem } from "./results-panel";

// Mock state values
let mockIsOpen = true;
let mockActiveTab = "problems";
const mockClose = vi.fn();
const mockToggle = vi.fn();

// Mock the results panel store
vi.mock("@/stores/results-panel-store", () => ({
  useResultsPanelStore: vi.fn((selector) => {
    const state = {
      isOpen: mockIsOpen,
      activeTab: mockActiveTab,
      close: mockClose,
      toggle: mockToggle,
      setProblemsCount: vi.fn(),
      problemsCount: 0,
      setActiveTab: vi.fn(),
      activeJobName: null,
    };
    return selector(state);
  }),
}));

// Mock JobLogsTab
vi.mock("./job-logs-tab", () => ({
  JobLogsTab: () => <div data-testid="job-logs-tab">No job selected. Select a job to view logs.</div>,
}));

// Mock JobResultsTab
vi.mock("./job-results-tab", () => ({
  JobResultsTab: () => <div data-testid="job-results-tab">No job selected. Run a job to view results.</div>,
}));

describe("ResultsPanel", () => {
  const mockProblems: Problem[] = [
    {
      severity: "error",
      message: "Test error",
      file: "test.yaml",
      line: 1,
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
    mockIsOpen = true;
    mockActiveTab = "problems";
  });

  it("should render when open", () => {
    mockIsOpen = true;
    render(<ResultsPanel problems={mockProblems} />);

    // Should show the problems tab content
    expect(screen.getByText("Test error")).toBeInTheDocument();
  });

  it("should render collapsed state when closed", () => {
    mockIsOpen = false;
    render(<ResultsPanel problems={mockProblems} />);

    // Should show the collapsed header
    expect(screen.getByText(/problems, logs, results/i)).toBeInTheDocument();
  });

  it("should call toggle when collapsed header is clicked", async () => {
    mockIsOpen = false;
    const user = userEvent.setup();
    render(<ResultsPanel problems={mockProblems} />);

    const header = screen.getByText(/problems, logs, results/i).closest("button");
    await user.click(header!);

    expect(mockToggle).toHaveBeenCalled();
  });

  it("should call close when close button is clicked", async () => {
    mockIsOpen = true;
    const user = userEvent.setup();
    render(<ResultsPanel problems={mockProblems} />);

    const closeButton = screen.getByTitle("Close panel");
    await user.click(closeButton);

    expect(mockClose).toHaveBeenCalled();
  });

  it("should call toggle when minimize button is clicked", async () => {
    mockIsOpen = true;
    const user = userEvent.setup();
    render(<ResultsPanel problems={mockProblems} />);

    const minimizeButton = screen.getByTitle("Minimize panel");
    await user.click(minimizeButton);

    expect(mockToggle).toHaveBeenCalled();
  });

  it("should call onProblemClick when problem is clicked", async () => {
    mockIsOpen = true;
    mockActiveTab = "problems";
    const user = userEvent.setup();
    const onProblemClick = vi.fn();
    render(<ResultsPanel problems={mockProblems} onProblemClick={onProblemClick} />);

    const problemButton = screen.getByText("Test error").closest("button");
    await user.click(problemButton!);

    expect(onProblemClick).toHaveBeenCalledWith(mockProblems[0]);
  });

  it("should render console content when provided", () => {
    mockIsOpen = true;
    mockActiveTab = "console";
    render(
      <ResultsPanel
        problems={[]}
        consoleContent={<div data-testid="custom-console">Custom Console</div>}
      />
    );

    expect(screen.getByTestId("custom-console")).toBeInTheDocument();
  });

  it("should show console placeholder when no content", () => {
    mockIsOpen = true;
    mockActiveTab = "console";
    render(<ResultsPanel problems={[]} />);

    expect(screen.getByText(/dev console/i)).toBeInTheDocument();
  });

  it("should apply custom className", () => {
    const { container } = render(
      <ResultsPanel problems={[]} className="custom-class" />
    );
    expect(container.firstChild).toHaveClass("custom-class");
  });

  it("should render logs tab content", () => {
    mockIsOpen = true;
    mockActiveTab = "logs";
    render(<ResultsPanel problems={[]} />);

    // JobLogsTab shows "No job selected" when no job
    expect(screen.getByText(/no job selected/i)).toBeInTheDocument();
  });

  it("should render results tab content", () => {
    mockIsOpen = true;
    mockActiveTab = "results";
    render(<ResultsPanel problems={[]} />);

    // JobResultsTab shows "No job selected" when no job
    expect(screen.getByText(/no job selected/i)).toBeInTheDocument();
  });

  it("should apply h-full class only when open", () => {
    mockIsOpen = true;
    const { container, rerender } = render(<ResultsPanel problems={[]} />);

    let panelContainer = container.querySelector("[data-results-panel-container]");
    expect(panelContainer).toHaveClass("h-full");

    // Re-render with closed state
    mockIsOpen = false;
    rerender(<ResultsPanel problems={[]} />);
    panelContainer = container.querySelector("[data-results-panel-container]");
    expect(panelContainer).not.toHaveClass("h-full");
  });
});

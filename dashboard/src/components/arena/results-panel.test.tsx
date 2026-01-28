import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ResultsPanel, type Problem } from "./results-panel";

// Mock state values
let mockIsOpen = true;
let mockActiveTab = "problems";
let mockHeight = 30;
const mockClose = vi.fn();
const mockToggle = vi.fn();
const mockSetHeight = vi.fn();

// Mock the results panel store
vi.mock("@/stores/results-panel-store", () => ({
  useResultsPanelStore: vi.fn((selector) => {
    const state = {
      isOpen: mockIsOpen,
      activeTab: mockActiveTab,
      height: mockHeight,
      close: mockClose,
      toggle: mockToggle,
      setHeight: mockSetHeight,
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
    mockHeight = 30;
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

  it("should handle keyboard navigation for resize", async () => {
    mockIsOpen = true;
    const user = userEvent.setup();
    render(<ResultsPanel problems={[]} />);

    // Find the resize handle by its role
    const resizeHandle = screen.getByRole("separator");

    // Focus the resize handle
    resizeHandle.focus();

    // Test ArrowUp (increase height)
    await user.keyboard("{ArrowUp}");
    expect(mockSetHeight).toHaveBeenCalledWith(35); // 30 + 5

    vi.clearAllMocks();

    // Test ArrowDown (decrease height)
    await user.keyboard("{ArrowDown}");
    expect(mockSetHeight).toHaveBeenCalledWith(25); // 30 - 5
  });

  it("should handle mouse resize interaction", async () => {
    mockIsOpen = true;
    render(<ResultsPanel problems={[]} />);

    const resizeHandle = screen.getByRole("separator");
    expect(resizeHandle).toBeInTheDocument();

    // Simulate mouse down
    const mouseDownEvent = new MouseEvent("mousedown", {
      bubbles: true,
      cancelable: true,
      clientY: 100,
    });
    resizeHandle.dispatchEvent(mouseDownEvent);

    // Simulate mouse move
    const mouseMoveEvent = new MouseEvent("mousemove", {
      bubbles: true,
      cancelable: true,
      clientY: 80, // moved up 20px
    });
    document.dispatchEvent(mouseMoveEvent);

    // Simulate mouse up
    const mouseUpEvent = new MouseEvent("mouseup", {
      bubbles: true,
      cancelable: true,
    });
    document.dispatchEvent(mouseUpEvent);

    // After resize interaction completes, the panel should still be rendered
    expect(screen.getByRole("separator")).toBeInTheDocument();
  });

  it("should set height to isOpen style correctly", () => {
    mockIsOpen = true;
    mockHeight = 40;
    const { container } = render(<ResultsPanel problems={[]} />);

    const panelContainer = container.querySelector("[data-results-panel-container]");
    expect(panelContainer).toHaveStyle({ height: "40%" });
  });

  it("should set height to auto when closed", () => {
    mockIsOpen = false;
    const { container } = render(<ResultsPanel problems={[]} />);

    const panelContainer = container.querySelector("[data-results-panel-container]");
    expect(panelContainer).toHaveStyle({ height: "auto" });
  });

  it("should show resize handle only when open", () => {
    mockIsOpen = true;
    render(<ResultsPanel problems={[]} />);
    expect(screen.getByRole("separator")).toBeInTheDocument();

    // Cleanup and re-render with closed state
    vi.clearAllMocks();
    mockIsOpen = false;
  });

  it("should not show resize handle when closed", () => {
    mockIsOpen = false;
    render(<ResultsPanel problems={[]} />);
    expect(screen.queryByRole("separator")).not.toBeInTheDocument();
  });
});

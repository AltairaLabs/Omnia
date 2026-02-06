import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ProblemsTab, ProblemsSummary, type Problem } from "./problems-tab";

// Mock the results panel store
vi.mock("@/stores/results-panel-store", () => ({
  useResultsPanelStore: vi.fn((selector) => {
    const state = {
      setProblemsCount: vi.fn(),
    };
    return selector(state);
  }),
}));

describe("ProblemsTab", () => {
  const mockProblems: Problem[] = [
    {
      severity: "error",
      message: "Missing required field",
      file: "agent.yaml",
      line: 10,
      column: 5,
      source: "yaml-lint",
    },
    {
      severity: "warning",
      message: "Deprecated field usage",
      file: "config.yaml",
      line: 25,
      source: "yaml-lint",
    },
    {
      severity: "info",
      message: "Consider using a shorter name",
      file: "prompts/main.txt",
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render empty state when no problems", () => {
    render(<ProblemsTab problems={[]} />);
    expect(screen.getByText(/no problems detected/i)).toBeInTheDocument();
  });

  it("should render problems list", () => {
    render(<ProblemsTab problems={mockProblems} />);

    expect(screen.getByText("Missing required field")).toBeInTheDocument();
    expect(screen.getByText("Deprecated field usage")).toBeInTheDocument();
    expect(screen.getByText("Consider using a shorter name")).toBeInTheDocument();
  });

  it("should display file headers", () => {
    render(<ProblemsTab problems={mockProblems} />);

    expect(screen.getByText("agent.yaml")).toBeInTheDocument();
    expect(screen.getByText("config.yaml")).toBeInTheDocument();
    expect(screen.getByText("prompts/main.txt")).toBeInTheDocument();
  });

  it("should display line and column info", () => {
    render(<ProblemsTab problems={mockProblems} />);

    expect(screen.getByText(/Ln 10, Col 5/)).toBeInTheDocument();
    expect(screen.getByText(/Ln 25/)).toBeInTheDocument();
  });

  it("should call onProblemClick when problem is clicked", async () => {
    const user = userEvent.setup();
    const onProblemClick = vi.fn();
    render(<ProblemsTab problems={mockProblems} onProblemClick={onProblemClick} />);

    const problemItem = screen.getByText("Missing required field").closest("button");
    await user.click(problemItem!);

    expect(onProblemClick).toHaveBeenCalledWith(mockProblems[0]);
  });

  it("should apply custom className", () => {
    const { container } = render(
      <ProblemsTab problems={mockProblems} className="custom-class" />
    );
    expect(container.firstChild).toHaveClass("custom-class");
  });

  it("should display source when available", () => {
    render(<ProblemsTab problems={mockProblems} />);
    expect(screen.getAllByText("(yaml-lint)").length).toBeGreaterThan(0);
  });

  it("should group problems by file", () => {
    const problemsSameFile: Problem[] = [
      { severity: "error", message: "Error 1", file: "test.yaml", line: 1 },
      { severity: "warning", message: "Warning 1", file: "test.yaml", line: 2 },
    ];
    render(<ProblemsTab problems={problemsSameFile} />);

    // Should have only one file header
    expect(screen.getAllByText("test.yaml")).toHaveLength(1);
    // Should show count
    expect(screen.getByText("(2)")).toBeInTheDocument();
  });
});

describe("ProblemsSummary", () => {
  it("should display error count", () => {
    const problems: Problem[] = [
      { severity: "error", message: "Error 1", file: "a.yaml" },
      { severity: "error", message: "Error 2", file: "b.yaml" },
    ];
    render(<ProblemsSummary problems={problems} />);
    expect(screen.getByText(/2 errors/i)).toBeInTheDocument();
  });

  it("should display warning count", () => {
    const problems: Problem[] = [
      { severity: "warning", message: "Warning 1", file: "a.yaml" },
    ];
    render(<ProblemsSummary problems={problems} />);
    expect(screen.getByText(/1 warning/i)).toBeInTheDocument();
  });

  it("should display info count", () => {
    const problems: Problem[] = [
      { severity: "info", message: "Info 1", file: "a.yaml" },
      { severity: "info", message: "Info 2", file: "b.yaml" },
    ];
    render(<ProblemsSummary problems={problems} />);
    expect(screen.getByText(/2 info/i)).toBeInTheDocument();
  });

  it("should show no problems message when empty", () => {
    render(<ProblemsSummary problems={[]} />);
    expect(screen.getByText(/no problems/i)).toBeInTheDocument();
  });

  it("should apply custom className", () => {
    const { container } = render(
      <ProblemsSummary problems={[]} className="custom-class" />
    );
    expect(container.firstChild).toHaveClass("custom-class");
  });

  it("should use singular form for single error", () => {
    const problems: Problem[] = [
      { severity: "error", message: "Error 1", file: "a.yaml" },
    ];
    render(<ProblemsSummary problems={problems} />);
    expect(screen.getByText("1 error")).toBeInTheDocument();
  });

  it("should use singular form for single warning", () => {
    const problems: Problem[] = [
      { severity: "warning", message: "Warning 1", file: "a.yaml" },
    ];
    render(<ProblemsSummary problems={problems} />);
    expect(screen.getByText("1 warning")).toBeInTheDocument();
  });
});

/**
 * Tests for QuickRunDialog component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QuickRunDialog, type QuickRunInitialValues } from "./quick-run-dialog";

// Mock hooks
vi.mock("@/hooks/use-agents", () => ({
  useAgents: () => ({
    data: [
      { metadata: { name: "agent-1" } },
      { metadata: { name: "agent-2" } },
    ],
  }),
}));

vi.mock("@/hooks/use-toast", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/hooks/use-project-jobs", () => ({
  useProjectJobsWithRun: () => ({
    deployed: true,
    running: false,
    run: vi.fn(),
  }),
}));

vi.mock("@/hooks/use-project-deployment", () => ({
  useProjectDeployment: () => ({
    status: { deployed: true },
    deploying: false,
    deploy: vi.fn(),
  }),
}));

describe("QuickRunDialog", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    projectId: "proj-123",
    type: "evaluation" as const,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders with default empty values when no initialValues provided", () => {
    render(<QuickRunDialog {...defaultProps} />);

    const nameInput = screen.getByLabelText("Job Name") as HTMLInputElement;
    expect(nameInput.value).toBe("");

    const includeInput = screen.getByLabelText("Include Scenarios") as HTMLInputElement;
    expect(includeInput.value).toBe("");

    const excludeInput = screen.getByLabelText("Exclude Scenarios") as HTMLInputElement;
    expect(excludeInput.value).toBe("");
  });

  it("pre-fills form fields when initialValues is provided", () => {
    const initialValues: QuickRunInitialValues = {
      name: "cloned-job",
      includePatterns: "scenarios/*.yaml",
      excludePatterns: "wip/**",
      verbose: true,
      executionMode: "direct",
    };

    render(<QuickRunDialog {...defaultProps} initialValues={initialValues} />);

    const nameInput = screen.getByLabelText("Job Name") as HTMLInputElement;
    expect(nameInput.value).toBe("cloned-job");

    const includeInput = screen.getByLabelText("Include Scenarios") as HTMLInputElement;
    expect(includeInput.value).toBe("scenarios/*.yaml");

    const excludeInput = screen.getByLabelText("Exclude Scenarios") as HTMLInputElement;
    expect(excludeInput.value).toBe("wip/**");
  });

  it("renders dialog title based on job type", () => {
    render(<QuickRunDialog {...defaultProps} type="loadtest" />);

    expect(screen.getAllByText("Run Load Test").length).toBeGreaterThan(0);
  });

  it("does not render when open is false", () => {
    render(<QuickRunDialog {...defaultProps} open={false} />);

    expect(screen.queryByText("Run Evaluation")).not.toBeInTheDocument();
  });
});

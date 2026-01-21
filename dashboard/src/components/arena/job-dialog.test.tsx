/**
 * Tests for JobDialog component.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { JobDialog } from "./job-dialog";

// Mock the mutations hook
vi.mock("@/hooks/use-arena-jobs", () => ({
  useArenaJobMutations: vi.fn(),
}));

const mockConfig = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1" as const,
  kind: "ArenaConfig" as const,
  metadata: { name: "test-config" },
  spec: { sourceRef: { name: "test-source" } },
  status: { phase: "Ready" as const },
};

describe("JobDialog", () => {
  const mockOnSuccess = vi.fn();
  const mockOnClose = vi.fn();
  const mockOnOpenChange = vi.fn();
  const mockCreateJob = vi.fn();

  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("renders create dialog correctly", async () => {
    const { useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: mockCreateJob,
      cancelJob: vi.fn(),
      deleteJob: vi.fn(),
      loading: false,
      error: null,
    });

    render(
      <JobDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        configs={[mockConfig]}
        onSuccess={mockOnSuccess}
        onClose={mockOnClose}
      />
    );

    expect(screen.getAllByText("Create Job").length).toBeGreaterThan(0);
    expect(screen.getByLabelText("Name")).toBeInTheDocument();
    expect(screen.getByLabelText("Config")).toBeInTheDocument();
    expect(screen.getByLabelText("Job Type")).toBeInTheDocument();
  });

  it("shows validation error for empty name", async () => {
    const { useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: mockCreateJob,
      cancelJob: vi.fn(),
      deleteJob: vi.fn(),
      loading: false,
      error: null,
    });

    render(
      <JobDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        configs={[mockConfig]}
        onSuccess={mockOnSuccess}
        onClose={mockOnClose}
      />
    );

    // Try to submit without filling name
    const createButtons = screen.getAllByRole("button", { name: /Create Job/i });
    const submitButton = createButtons[createButtons.length - 1]; // Last one is the submit button
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText("Name is required")).toBeInTheDocument();
    });
  });

  it("shows validation error for invalid name format", async () => {
    const { useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: mockCreateJob,
      cancelJob: vi.fn(),
      deleteJob: vi.fn(),
      loading: false,
      error: null,
    });

    render(
      <JobDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        configs={[mockConfig]}
        onSuccess={mockOnSuccess}
        onClose={mockOnClose}
      />
    );

    // Enter invalid name
    const nameInput = screen.getByLabelText("Name");
    fireEvent.change(nameInput, { target: { value: "Invalid_Name!" } });

    // Try to submit
    const createButtons = screen.getAllByRole("button", { name: /Create Job/i });
    const submitButton = createButtons[createButtons.length - 1];
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText("Name must be lowercase alphanumeric and may contain hyphens")).toBeInTheDocument();
    });
  });

  it("shows workers and timeout fields", async () => {
    const { useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: mockCreateJob,
      cancelJob: vi.fn(),
      deleteJob: vi.fn(),
      loading: false,
      error: null,
    });

    render(
      <JobDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        configs={[mockConfig]}
        onSuccess={mockOnSuccess}
        onClose={mockOnClose}
      />
    );

    expect(screen.getByLabelText("Workers")).toBeInTheDocument();
    expect(screen.getByLabelText("Timeout")).toBeInTheDocument();
  });

  it("shows evaluation options by default", async () => {
    const { useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: mockCreateJob,
      cancelJob: vi.fn(),
      deleteJob: vi.fn(),
      loading: false,
      error: null,
    });

    render(
      <JobDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        configs={[mockConfig]}
        onSuccess={mockOnSuccess}
        onClose={mockOnClose}
      />
    );

    expect(screen.getByText("Evaluation Options")).toBeInTheDocument();
    expect(screen.getByText("Passing Threshold")).toBeInTheDocument();
    expect(screen.getByText("Continue on Failure")).toBeInTheDocument();
  });

  it("preselects config when preselectedConfig is provided", async () => {
    const { useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: mockCreateJob,
      cancelJob: vi.fn(),
      deleteJob: vi.fn(),
      loading: false,
      error: null,
    });

    render(
      <JobDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        configs={[mockConfig]}
        preselectedConfig="test-config"
        onSuccess={mockOnSuccess}
        onClose={mockOnClose}
      />
    );

    // The config should be preselected
    expect(screen.getByText("test-config")).toBeInTheDocument();
  });

  it("closes dialog when cancel is clicked", async () => {
    const { useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: mockCreateJob,
      cancelJob: vi.fn(),
      deleteJob: vi.fn(),
      loading: false,
      error: null,
    });

    render(
      <JobDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        configs={[mockConfig]}
        onSuccess={mockOnSuccess}
        onClose={mockOnClose}
      />
    );

    const cancelButton = screen.getByText("Cancel");
    fireEvent.click(cancelButton);

    expect(mockOnClose).toHaveBeenCalled();
    expect(mockOnOpenChange).toHaveBeenCalledWith(false);
  });

  it("shows no configs message when no ready configs", async () => {
    const { useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: mockCreateJob,
      cancelJob: vi.fn(),
      deleteJob: vi.fn(),
      loading: false,
      error: null,
    });

    const pendingConfig = {
      ...mockConfig,
      status: { phase: "Pending" as const },
    };

    render(
      <JobDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        configs={[pendingConfig]}
        onSuccess={mockOnSuccess}
        onClose={mockOnClose}
      />
    );

    // Open the config dropdown
    const configTrigger = screen.getByLabelText("Config");
    fireEvent.click(configTrigger);

    await waitFor(() => {
      expect(screen.getByText("No ready configs available")).toBeInTheDocument();
    });
  });

  it("submits form with correct data", async () => {
    const { useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    mockCreateJob.mockResolvedValue({
      metadata: { name: "my-job" },
      spec: { configRef: { name: "test-config" }, type: "evaluation", workers: { replicas: 2 } },
    });

    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: mockCreateJob,
      cancelJob: vi.fn(),
      deleteJob: vi.fn(),
      loading: false,
      error: null,
    });

    render(
      <JobDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        configs={[mockConfig]}
        preselectedConfig="test-config"
        onSuccess={mockOnSuccess}
        onClose={mockOnClose}
      />
    );

    // Fill in the name
    const nameInput = screen.getByLabelText("Name");
    fireEvent.change(nameInput, { target: { value: "my-job" } });

    // Submit the form
    const createButtons = screen.getAllByRole("button", { name: /Create Job/i });
    const createButton = createButtons[createButtons.length - 1];
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(mockCreateJob).toHaveBeenCalledWith("my-job", expect.objectContaining({
        configRef: { name: "test-config" },
        type: "evaluation",
        workers: { replicas: 2 },
      }));
    });
  });

  it("shows error when create fails", async () => {
    const { useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    mockCreateJob.mockRejectedValue(new Error("Job already exists"));

    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: mockCreateJob,
      cancelJob: vi.fn(),
      deleteJob: vi.fn(),
      loading: false,
      error: null,
    });

    render(
      <JobDialog
        open={true}
        onOpenChange={mockOnOpenChange}
        configs={[mockConfig]}
        preselectedConfig="test-config"
        onSuccess={mockOnSuccess}
        onClose={mockOnClose}
      />
    );

    // Fill in the name
    const nameInput = screen.getByLabelText("Name");
    fireEvent.change(nameInput, { target: { value: "my-job" } });

    // Submit the form
    const createButtons = screen.getAllByRole("button", { name: /Create Job/i });
    const createButton = createButtons[createButtons.length - 1];
    fireEvent.click(createButton);

    await waitFor(() => {
      expect(screen.getByText("Job already exists")).toBeInTheDocument();
    });
  });
});

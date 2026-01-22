/**
 * Tests for Arena JobDialog component
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { JobDialog } from "./job-dialog";
import type { ArenaConfig } from "@/types/arena";

// Mock the hooks
const mockCreateJob = vi.fn();
vi.mock("@/hooks/use-arena-jobs", () => ({
  useArenaJobMutations: () => ({
    createJob: mockCreateJob,
    loading: false,
  }),
}));

// Mock license hook with configurable values
let mockIsEnterprise = true;
let mockMaxWorkerReplicas = 0;
vi.mock("@/hooks/use-license", () => ({
  useLicense: () => ({
    isEnterprise: mockIsEnterprise,
    license: {
      limits: {
        maxWorkerReplicas: mockMaxWorkerReplicas,
      },
    },
  }),
}));

// Helper to create mock configs
function createMockConfig(name: string, phase: string = "Ready"): ArenaConfig {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ArenaConfig",
    metadata: { name },
    spec: { sourceRef: { name: "test-source" } },
    status: { phase: phase as "Pending" | "Ready" | "Failed" },
  };
}

describe("JobDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateJob.mockResolvedValue({ metadata: { name: "test-job" } });
    // Reset license mocks to enterprise defaults
    mockIsEnterprise = true;
    mockMaxWorkerReplicas = 0;
  });

  describe("rendering", () => {
    it("renders dialog when open", () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      // Dialog title and submit button both have "Create Job" text
      expect(screen.getByRole("heading", { name: "Create Job" })).toBeInTheDocument();
      expect(screen.getByLabelText("Name")).toBeInTheDocument();
      expect(screen.getByLabelText("Config")).toBeInTheDocument();
      expect(screen.getByLabelText("Job Type")).toBeInTheDocument();
    });

    it("does not render dialog when closed", () => {
      render(
        <JobDialog
          open={false}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      expect(screen.queryByText("Create Job")).not.toBeInTheDocument();
    });

    it("shows only ready configs in dropdown", () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[
            createMockConfig("ready-config", "Ready"),
            createMockConfig("pending-config", "Pending"),
            createMockConfig("failed-config", "Failed"),
          ]}
        />
      );

      // Open the config select
      fireEvent.click(screen.getByLabelText("Config"));

      // Only ready config should be visible
      expect(screen.getByRole("option", { name: "ready-config" })).toBeInTheDocument();
      expect(screen.queryByRole("option", { name: "pending-config" })).not.toBeInTheDocument();
      expect(screen.queryByRole("option", { name: "failed-config" })).not.toBeInTheDocument();
    });

    it("shows message when no ready configs available", () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("pending-config", "Pending")]}
        />
      );

      fireEvent.click(screen.getByLabelText("Config"));
      expect(screen.getByText("No ready configs available")).toBeInTheDocument();
    });

    it("preselects config when provided", () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("preselected-config")]}
          preselectedConfig="preselected-config"
        />
      );

      expect(screen.getByLabelText("Config")).toHaveTextContent("preselected-config");
    });
  });

  describe("job type options", () => {
    it("shows evaluation options by default", () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      expect(screen.getByText("Evaluation Options")).toBeInTheDocument();
      expect(screen.getByText("Passing Threshold")).toBeInTheDocument();
      expect(screen.getByText("Continue on Failure")).toBeInTheDocument();
    });

    it("shows load test options when loadtest type selected", async () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      // Change job type to loadtest
      fireEvent.click(screen.getByLabelText("Job Type"));
      fireEvent.click(screen.getByRole("option", { name: "Load Test" }));

      await waitFor(() => {
        expect(screen.getByText("Load Test Options")).toBeInTheDocument();
      });
      expect(screen.getByText("Profile Type")).toBeInTheDocument();
      expect(screen.getByText("Duration")).toBeInTheDocument();
      expect(screen.getByText("Target RPS")).toBeInTheDocument();
    });

    it("shows data generation options when datagen type selected", async () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      // Change job type to datagen
      fireEvent.click(screen.getByLabelText("Job Type"));
      fireEvent.click(screen.getByRole("option", { name: "Data Generation" }));

      await waitFor(() => {
        expect(screen.getByText("Data Generation Options")).toBeInTheDocument();
      });
      expect(screen.getByText("Sample Count")).toBeInTheDocument();
      expect(screen.getByText("Deduplicate")).toBeInTheDocument();
    });
  });

  describe("form validation", () => {
    it("shows error when name is empty", async () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(screen.getByText("Name is required")).toBeInTheDocument();
      });
      expect(mockCreateJob).not.toHaveBeenCalled();
    });

    it("shows error for invalid name format", async () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Invalid_Name" } });
      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(screen.getByText("Name must be lowercase alphanumeric and may contain hyphens")).toBeInTheDocument();
      });
    });

    it("shows error when config is not selected", async () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "valid-name" } });
      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(screen.getByText("Config is required")).toBeInTheDocument();
      });
    });

    it("shows error for invalid workers value", async () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
        />
      );

      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "valid-name" } });
      fireEvent.change(screen.getByLabelText("Workers"), { target: { value: "0" } });
      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(screen.getByText("Workers must be a positive integer")).toBeInTheDocument();
      });
    });

    it("shows error for invalid passing threshold", async () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
        />
      );

      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "valid-name" } });
      fireEvent.change(screen.getByLabelText("Passing Threshold"), { target: { value: "1.5" } });
      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(screen.getByText("Passing threshold must be a number between 0 and 1")).toBeInTheDocument();
      });
    });

    it("shows error for invalid target RPS in loadtest", async () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
        />
      );

      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "valid-name" } });

      // Change to loadtest
      fireEvent.click(screen.getByLabelText("Job Type"));
      fireEvent.click(screen.getByRole("option", { name: "Load Test" }));

      await waitFor(() => {
        expect(screen.getByLabelText("Target RPS")).toBeInTheDocument();
      });

      fireEvent.change(screen.getByLabelText("Target RPS"), { target: { value: "0" } });
      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(screen.getByText("Target RPS must be a positive integer")).toBeInTheDocument();
      });
    });

    it("shows error for invalid sample count in datagen", async () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
        />
      );

      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "valid-name" } });

      // Change to datagen
      fireEvent.click(screen.getByLabelText("Job Type"));
      fireEvent.click(screen.getByRole("option", { name: "Data Generation" }));

      await waitFor(() => {
        expect(screen.getByLabelText("Sample Count")).toBeInTheDocument();
      });

      fireEvent.change(screen.getByLabelText("Sample Count"), { target: { value: "-1" } });
      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(screen.getByText("Sample count must be a positive integer")).toBeInTheDocument();
      });
    });
  });

  describe("form submission", () => {
    it("creates evaluation job with correct spec", async () => {
      const onSuccess = vi.fn();
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
          onSuccess={onSuccess}
        />
      );

      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "my-eval-job" } });
      fireEvent.change(screen.getByLabelText("Workers"), { target: { value: "4" } });
      fireEvent.change(screen.getByLabelText("Timeout"), { target: { value: "1h" } });
      fireEvent.change(screen.getByLabelText("Passing Threshold"), { target: { value: "0.9" } });

      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(mockCreateJob).toHaveBeenCalledWith("my-eval-job", {
          configRef: { name: "test-config" },
          type: "evaluation",
          workers: { replicas: 4 },
          timeout: "1h",
          evaluation: {
            continueOnFailure: true,
            passingThreshold: 0.9,
            outputFormats: ["json", "junit"],
          },
        });
      });
      expect(onSuccess).toHaveBeenCalled();
    });

    it("creates loadtest job with correct spec", async () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
        />
      );

      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "my-loadtest" } });

      // Change to loadtest
      fireEvent.click(screen.getByLabelText("Job Type"));
      fireEvent.click(screen.getByRole("option", { name: "Load Test" }));

      await waitFor(() => {
        expect(screen.getByLabelText("Target RPS")).toBeInTheDocument();
      });

      fireEvent.change(screen.getByLabelText("Target RPS"), { target: { value: "50" } });
      fireEvent.change(screen.getByLabelText("Duration"), { target: { value: "10m" } });

      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(mockCreateJob).toHaveBeenCalledWith("my-loadtest", expect.objectContaining({
          type: "loadtest",
          loadtest: {
            profileType: "constant",
            duration: "10m",
            targetRPS: 50,
          },
        }));
      });
    });

    it("creates datagen job with correct spec", async () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
        />
      );

      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "my-datagen" } });

      // Change to datagen
      fireEvent.click(screen.getByLabelText("Job Type"));
      fireEvent.click(screen.getByRole("option", { name: "Data Generation" }));

      await waitFor(() => {
        expect(screen.getByLabelText("Sample Count")).toBeInTheDocument();
      });

      fireEvent.change(screen.getByLabelText("Sample Count"), { target: { value: "500" } });

      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(mockCreateJob).toHaveBeenCalledWith("my-datagen", expect.objectContaining({
          type: "datagen",
          datagen: {
            sampleCount: 500,
            deduplicate: true,
            outputFormat: "jsonl",
          },
        }));
      });
    });

    it("shows error when createJob fails", async () => {
      mockCreateJob.mockRejectedValue(new Error("API Error"));

      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
        />
      );

      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "my-job" } });
      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(screen.getByText("API Error")).toBeInTheDocument();
      });
    });

    it("shows generic error for non-Error exceptions", async () => {
      mockCreateJob.mockRejectedValue("Unknown failure");

      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
        />
      );

      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "my-job" } });
      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(screen.getByText("Failed to create job")).toBeInTheDocument();
      });
    });
  });

  describe("dialog actions", () => {
    it("calls onClose and onOpenChange when cancel is clicked", () => {
      const onClose = vi.fn();
      const onOpenChange = vi.fn();

      render(
        <JobDialog
          open={true}
          onOpenChange={onOpenChange}
          configs={[createMockConfig("test-config")]}
          onClose={onClose}
        />
      );

      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

      expect(onClose).toHaveBeenCalled();
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });
  });

  describe("form interactions", () => {
    it("updates workers field", () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      const workersInput = screen.getByLabelText("Workers");
      fireEvent.change(workersInput, { target: { value: "8" } });
      expect(workersInput).toHaveValue(8);
    });

    it("updates timeout field", () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      const timeoutInput = screen.getByLabelText("Timeout");
      fireEvent.change(timeoutInput, { target: { value: "2h" } });
      expect(timeoutInput).toHaveValue("2h");
    });

    it("toggles continue on failure switch", () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      // Get the continue on failure switch (second switch - after verbose switch)
      const switches = screen.getAllByRole("switch");
      const continueOnFailureSwitch = switches[1]; // switches[0] is verbose, switches[1] is continue on failure
      expect(continueOnFailureSwitch).toBeChecked();

      fireEvent.click(continueOnFailureSwitch);
      expect(continueOnFailureSwitch).not.toBeChecked();
    });

    it("changes profile type in loadtest", async () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
        />
      );

      // Change to loadtest
      fireEvent.click(screen.getByLabelText("Job Type"));
      fireEvent.click(screen.getByRole("option", { name: "Load Test" }));

      await waitFor(() => {
        expect(screen.getByLabelText("Profile Type")).toBeInTheDocument();
      });

      fireEvent.click(screen.getByLabelText("Profile Type"));
      fireEvent.click(screen.getByRole("option", { name: "Ramp" }));

      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "ramp-test" } });
      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(mockCreateJob).toHaveBeenCalledWith("ramp-test", expect.objectContaining({
          loadtest: expect.objectContaining({
            profileType: "ramp",
          }),
        }));
      });
    });

    it("toggles deduplicate switch in datagen", async () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
        />
      );

      // Change to datagen
      fireEvent.click(screen.getByLabelText("Job Type"));
      fireEvent.click(screen.getByRole("option", { name: "Data Generation" }));

      await waitFor(() => {
        expect(screen.getByText("Data Generation Options")).toBeInTheDocument();
      });

      // Find deduplicate switch (second switch on page - after verbose switch)
      const switches = screen.getAllByRole("switch");
      const deduplicateSwitch = switches[1]; // switches[0] is verbose, switches[1] is deduplicate

      expect(deduplicateSwitch).toBeChecked();
      fireEvent.click(deduplicateSwitch);
      expect(deduplicateSwitch).not.toBeChecked();
    });
  });

  describe("edge cases", () => {
    it("handles empty timeout (uses undefined)", async () => {
      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
        />
      );

      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "no-timeout-job" } });
      fireEvent.change(screen.getByLabelText("Timeout"), { target: { value: "" } });
      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(mockCreateJob).toHaveBeenCalledWith("no-timeout-job", expect.objectContaining({
          timeout: undefined,
        }));
      });
    });

    it("handles config with undefined metadata name", () => {
      // Use type assertion to test defensive UI code path when runtime data doesn't match types
      const configWithNoName = {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "ArenaConfig",
        metadata: {} as { name?: string },
        spec: { sourceRef: { name: "test" } },
        status: { phase: "Ready" },
      } as ArenaConfig;

      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[configWithNoName]}
        />
      );

      fireEvent.click(screen.getByLabelText("Config"));
      // Config displays metadata.name (empty) but uses "unknown" as value
      // Verify the listbox is visible and has an option
      const listbox = screen.getByRole("listbox");
      expect(listbox).toBeInTheDocument();
      // There should be an option element (even with empty name)
      const options = screen.getAllByRole("option");
      expect(options.length).toBeGreaterThan(0);
    });
  });

  describe("license gating", () => {
    it("disables loadtest and datagen for open core users", () => {
      mockIsEnterprise = false;

      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      fireEvent.click(screen.getByLabelText("Job Type"));

      // Evaluation should be enabled
      const evaluationOption = screen.getByRole("option", { name: /Evaluation/i });
      expect(evaluationOption).not.toHaveAttribute("data-disabled");

      // Load Test should be disabled with Enterprise badge
      const loadTestOption = screen.getByRole("option", { name: /Load Test.*Enterprise/i });
      expect(loadTestOption).toHaveAttribute("data-disabled");

      // Data Generation should be disabled with Enterprise badge
      const dataGenOption = screen.getByRole("option", { name: /Data Generation.*Enterprise/i });
      expect(dataGenOption).toHaveAttribute("data-disabled");
    });

    it("enables all job types for enterprise users", () => {
      mockIsEnterprise = true;

      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      fireEvent.click(screen.getByLabelText("Job Type"));

      // All options should be enabled for enterprise
      const evaluationOption = screen.getByRole("option", { name: /Evaluation/i });
      expect(evaluationOption).not.toHaveAttribute("data-disabled");

      const loadTestOption = screen.getByRole("option", { name: /Load Test/i });
      expect(loadTestOption).not.toHaveAttribute("data-disabled");

      const dataGenOption = screen.getByRole("option", { name: /Data Generation/i });
      expect(dataGenOption).not.toHaveAttribute("data-disabled");
    });

    it("shows worker limit warning for open core users", () => {
      mockIsEnterprise = false;
      mockMaxWorkerReplicas = 1;

      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      expect(screen.getByText(/Limited to 1 worker/i)).toBeInTheDocument();
      expect(screen.getByText(/upgrade for more/i)).toBeInTheDocument();
    });

    it("does not show worker limit warning for enterprise users", () => {
      mockIsEnterprise = true;
      mockMaxWorkerReplicas = 0;

      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      expect(screen.queryByText(/Limited to/i)).not.toBeInTheDocument();
    });

    it("shows validation error when workers exceed limit", async () => {
      mockIsEnterprise = false;
      mockMaxWorkerReplicas = 1;

      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
        />
      );

      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "test-job" } });
      fireEvent.change(screen.getByLabelText("Workers"), { target: { value: "5" } });
      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(screen.getByText(/Open Core is limited to 1 worker/i)).toBeInTheDocument();
      });
      expect(mockCreateJob).not.toHaveBeenCalled();
    });

    it("allows creating job with workers within limit", async () => {
      mockIsEnterprise = false;
      mockMaxWorkerReplicas = 1;

      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
        />
      );

      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "test-job" } });
      fireEvent.change(screen.getByLabelText("Workers"), { target: { value: "1" } });
      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(mockCreateJob).toHaveBeenCalled();
      });
    });
  });
});

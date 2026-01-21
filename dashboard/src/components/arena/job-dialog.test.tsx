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

// Mock the license hook with configurable values
const mockUseLicense = vi.fn();
vi.mock("@/hooks/use-license", () => ({
  useLicense: () => mockUseLicense(),
}));

// Enterprise license mock
const enterpriseLicense = {
  license: {
    id: "enterprise-test",
    tier: "enterprise",
    customer: "Test Corp",
    features: {
      gitSource: true,
      ociSource: true,
      s3Source: true,
      loadTesting: true,
      dataGeneration: true,
      scheduling: true,
      distributedWorkers: true,
    },
    limits: {
      maxScenarios: 0, // unlimited
      maxWorkerReplicas: 0, // unlimited
    },
    issuedAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toISOString(),
  },
  isEnterprise: true,
  isLoading: false,
  error: undefined,
  canUseFeature: () => true,
  canUseSourceType: () => true,
  canUseJobType: () => true,
  canUseScheduling: () => true,
  canUseWorkerReplicas: () => true,
  canUseScenarioCount: () => true,
  isExpired: false,
  refresh: vi.fn(),
};

// Open Core license mock
const openCoreLicense = {
  license: {
    id: "open-core",
    tier: "open-core",
    customer: "Open Core User",
    features: {
      gitSource: false,
      ociSource: false,
      s3Source: false,
      loadTesting: false,
      dataGeneration: false,
      scheduling: false,
      distributedWorkers: false,
    },
    limits: {
      maxScenarios: 10,
      maxWorkerReplicas: 1,
    },
    issuedAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toISOString(),
  },
  isEnterprise: false,
  isLoading: false,
  error: undefined,
  canUseFeature: () => false,
  canUseSourceType: (type: string) => type === "configmap",
  canUseJobType: (type: string) => type === "evaluation",
  canUseScheduling: () => false,
  canUseWorkerReplicas: (replicas: number) => replicas <= 1,
  canUseScenarioCount: (count: number) => count <= 10,
  isExpired: false,
  refresh: vi.fn(),
};

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
    // Default to enterprise license for most tests (backward compatibility)
    mockUseLicense.mockReturnValue(enterpriseLicense);
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

      const switchElement = screen.getByRole("switch");
      expect(switchElement).toBeChecked();

      fireEvent.click(switchElement);
      expect(switchElement).not.toBeChecked();
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

      // Find deduplicate switch (second switch on page)
      const switches = screen.getAllByRole("switch");
      const deduplicateSwitch = switches[0]; // Only switch visible in datagen mode

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
    it("disables loadtest and datagen job types for Open Core users", () => {
      mockUseLicense.mockReturnValue(openCoreLicense);

      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      // Open job type dropdown
      fireEvent.click(screen.getByLabelText("Job Type"));

      // Evaluation should be enabled
      const evaluationOption = screen.getByRole("option", { name: /Evaluation/ });
      expect(evaluationOption).not.toHaveAttribute("data-disabled");

      // Load Test and Data Generation should be disabled with Enterprise badge
      const loadTestOption = screen.getByRole("option", { name: /Load Test/ });
      expect(loadTestOption).toHaveAttribute("data-disabled");

      const dataGenOption = screen.getByRole("option", { name: /Data Generation/ });
      expect(dataGenOption).toHaveAttribute("data-disabled");

      // Both enterprise options should have Enterprise badges
      const enterpriseBadges = screen.getAllByText("Enterprise");
      expect(enterpriseBadges).toHaveLength(2);
    });

    it("shows worker limit message for Open Core users", () => {
      mockUseLicense.mockReturnValue(openCoreLicense);

      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      expect(screen.getByText(/Limited to 1 worker/)).toBeInTheDocument();
    });

    it("does not show worker limit message for Enterprise users", () => {
      mockUseLicense.mockReturnValue(enterpriseLicense);

      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      expect(screen.queryByText(/Limited to.*worker/)).not.toBeInTheDocument();
    });

    it("shows validation error when submitting enterprise job type as Open Core user", async () => {
      // Start with enterprise to allow selecting loadtest
      mockUseLicense.mockReturnValue(enterpriseLicense);

      const { rerender } = render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
        />
      );

      // Fill in name and select loadtest
      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "my-loadtest" } });
      fireEvent.click(screen.getByLabelText("Job Type"));
      fireEvent.click(screen.getByRole("option", { name: /Load Test/ }));

      // Switch to open core license
      mockUseLicense.mockReturnValue(openCoreLicense);
      rerender(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
        />
      );

      // Try to submit
      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(screen.getByText(/Load Test requires an Enterprise license/)).toBeInTheDocument();
      });
    });

    it("shows validation error when worker count exceeds limit", async () => {
      mockUseLicense.mockReturnValue(openCoreLicense);

      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
          preselectedConfig="test-config"
        />
      );

      fireEvent.change(screen.getByLabelText("Name"), { target: { value: "my-job" } });
      fireEvent.change(screen.getByLabelText("Workers"), { target: { value: "5" } });
      fireEvent.click(screen.getByRole("button", { name: "Create Job" }));

      await waitFor(() => {
        expect(screen.getByText(/Open Core is limited to 1 worker/)).toBeInTheDocument();
      });
    });

    it("shows scenario warning when config exceeds limit", () => {
      mockUseLicense.mockReturnValue(openCoreLicense);

      // Create config with high scenario count
      const configWithManyScenarios: ArenaConfig = {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "ArenaConfig",
        metadata: { name: "many-scenarios" },
        spec: { sourceRef: { name: "test-source" } },
        status: { phase: "Ready", scenarioCount: 25 },
      };

      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[configWithManyScenarios]}
          preselectedConfig="many-scenarios"
        />
      );

      expect(screen.getByText(/This config has 25 scenarios/)).toBeInTheDocument();
      expect(screen.getByText(/Open Core is limited to 10 scenarios/)).toBeInTheDocument();
    });

    it("does not show scenario warning for Enterprise users", () => {
      mockUseLicense.mockReturnValue(enterpriseLicense);

      const configWithManyScenarios: ArenaConfig = {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "ArenaConfig",
        metadata: { name: "many-scenarios" },
        spec: { sourceRef: { name: "test-source" } },
        status: { phase: "Ready", scenarioCount: 25 },
      };

      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[configWithManyScenarios]}
          preselectedConfig="many-scenarios"
        />
      );

      expect(screen.queryByText(/This config has 25 scenarios/)).not.toBeInTheDocument();
    });

    it("enables all job types for Enterprise users", () => {
      mockUseLicense.mockReturnValue(enterpriseLicense);

      render(
        <JobDialog
          open={true}
          onOpenChange={vi.fn()}
          configs={[createMockConfig("test-config")]}
        />
      );

      fireEvent.click(screen.getByLabelText("Job Type"));

      // All options should be enabled
      const options = screen.getAllByRole("option");
      options.forEach((option) => {
        expect(option).not.toHaveAttribute("data-disabled");
      });
    });
  });
});

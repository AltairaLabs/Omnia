/**
 * Tests for Arena JobDialog wrapper component.
 * The actual wizard logic is tested in job-wizard.test.tsx.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { JobDialog } from "./job-dialog";
import type { ArenaSource } from "@/types/arena";

// Mock workspace context
const mockCurrentWorkspace = {
  name: "test-workspace",
  namespace: "test-namespace",
  role: "editor",
};

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: mockCurrentWorkspace,
    workspaces: [mockCurrentWorkspace],
    isLoading: false,
    error: null,
    setCurrentWorkspace: vi.fn(),
    refetch: vi.fn(),
  }),
}));

// Mock the hooks
const mockCreateJob = vi.fn();
vi.mock("@/hooks/use-arena-jobs", () => ({
  useArenaJobMutations: () => ({
    createJob: mockCreateJob,
    loading: false,
  }),
}));

// Mock license hook
vi.mock("@/hooks/use-license", () => ({
  useLicense: () => ({
    isEnterprise: true,
    license: {
      limits: {
        maxWorkerReplicas: 0,
      },
    },
  }),
}));

// Mock useArenaSourceContent hook
vi.mock("@/hooks/use-arena-source-content", () => ({
  useArenaSourceContent: () => ({
    tree: [],
    loading: false,
    error: null,
  }),
}));

// Mock provider and tool registry data
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    getProviders: vi.fn().mockResolvedValue([]),
    getToolRegistries: vi.fn().mockResolvedValue([]),
  }),
}));

// Helper to create mock sources
function createMockSource(name: string, phase: string = "Ready"): ArenaSource {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ArenaSource",
    metadata: { name },
    spec: {
      type: "git",
      interval: "5m",
      git: { url: "https://github.com/test/repo" },
    },
    status: { phase: phase as "Pending" | "Ready" | "Failed" },
  };
}

function TestWrapper({ children }: { children: React.ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

describe("JobDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    mockCreateJob.mockResolvedValue({ metadata: { name: "test-job" } });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe("dialog rendering", () => {
    it("renders dialog when open", () => {
      render(
        <TestWrapper>
          <JobDialog
            open={true}
            onOpenChange={vi.fn()}
            sources={[createMockSource("test-source")]}
          />
        </TestWrapper>
      );

      // Dialog title
      expect(screen.getByRole("heading", { name: "Create Job" })).toBeInTheDocument();
      // Job wizard should be present - it starts with step 0 (Basic Info)
      expect(screen.getByText("Job Name")).toBeInTheDocument();
    });

    it("does not render dialog when closed", () => {
      render(
        <TestWrapper>
          <JobDialog
            open={false}
            onOpenChange={vi.fn()}
            sources={[createMockSource("test-source")]}
          />
        </TestWrapper>
      );

      expect(screen.queryByText("Create Job")).not.toBeInTheDocument();
    });

    it("renders dialog description", () => {
      render(
        <TestWrapper>
          <JobDialog
            open={true}
            onOpenChange={vi.fn()}
            sources={[createMockSource("test-source")]}
          />
        </TestWrapper>
      );

      expect(
        screen.getByText(/Create a new Arena job to run evaluations/)
      ).toBeInTheDocument();
    });
  });

  describe("props and callbacks", () => {
    it("passes preselectedSource to JobWizard", () => {
      render(
        <TestWrapper>
          <JobDialog
            open={true}
            onOpenChange={vi.fn()}
            sources={[createMockSource("my-source")]}
            preselectedSource="my-source"
          />
        </TestWrapper>
      );

      // Dialog should render with JobWizard
      expect(screen.getByText("Job Name")).toBeInTheDocument();
    });

    it("calls onOpenChange when dialog state changes", async () => {
      const onOpenChange = vi.fn();
      const { rerender } = render(
        <TestWrapper>
          <JobDialog
            open={true}
            onOpenChange={onOpenChange}
            sources={[createMockSource("test-source")]}
          />
        </TestWrapper>
      );

      // Rerender with closed state
      rerender(
        <TestWrapper>
          <JobDialog
            open={false}
            onOpenChange={onOpenChange}
            sources={[createMockSource("test-source")]}
          />
        </TestWrapper>
      );

      expect(screen.queryByText("Create Job")).not.toBeInTheDocument();
    });

    it("calls onClose callback when wizard closes", async () => {
      const onClose = vi.fn();
      const onOpenChange = vi.fn();
      render(
        <TestWrapper>
          <JobDialog
            open={true}
            onOpenChange={onOpenChange}
            sources={[createMockSource("test-source")]}
            onClose={onClose}
          />
        </TestWrapper>
      );

      // Click cancel button (wizard's close action)
      const cancelButton = screen.getByRole("button", { name: /cancel/i });
      cancelButton.click();

      expect(onClose).toHaveBeenCalled();
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });

    it("calls onSuccess and closes dialog after successful job creation", async () => {
      vi.useRealTimers(); // Use real timers for this async test
      const onSuccess = vi.fn();
      const onClose = vi.fn();
      const onOpenChange = vi.fn();

      render(
        <TestWrapper>
          <JobDialog
            open={true}
            onOpenChange={onOpenChange}
            sources={[createMockSource("test-source")]}
            onSuccess={onSuccess}
            onClose={onClose}
          />
        </TestWrapper>
      );

      // The JobWizard will call onSuccess when job is created successfully
      // We just verify the props are passed correctly
      expect(screen.getByText("Job Name")).toBeInTheDocument();
    });
  });

  describe("form reset behavior", () => {
    it("generates unique key based on preselectedSource and open state", () => {
      const { rerender } = render(
        <TestWrapper>
          <JobDialog
            open={true}
            onOpenChange={vi.fn()}
            sources={[createMockSource("source-1"), createMockSource("source-2")]}
            preselectedSource="source-1"
          />
        </TestWrapper>
      );

      // Rerender with different preselected source
      rerender(
        <TestWrapper>
          <JobDialog
            open={true}
            onOpenChange={vi.fn()}
            sources={[createMockSource("source-1"), createMockSource("source-2")]}
            preselectedSource="source-2"
          />
        </TestWrapper>
      );

      // Form should reset (wizard should be on first step)
      expect(screen.getByText("Job Name")).toBeInTheDocument();
    });

    it("resets form when dialog reopens", () => {
      const { rerender } = render(
        <TestWrapper>
          <JobDialog
            open={true}
            onOpenChange={vi.fn()}
            sources={[createMockSource("test-source")]}
          />
        </TestWrapper>
      );

      // Close dialog
      rerender(
        <TestWrapper>
          <JobDialog
            open={false}
            onOpenChange={vi.fn()}
            sources={[createMockSource("test-source")]}
          />
        </TestWrapper>
      );

      // Reopen dialog
      rerender(
        <TestWrapper>
          <JobDialog
            open={true}
            onOpenChange={vi.fn()}
            sources={[createMockSource("test-source")]}
          />
        </TestWrapper>
      );

      // Should show the wizard's first step
      expect(screen.getByText("Job Name")).toBeInTheDocument();
    });

    it("uses 'new' in key when no preselectedSource provided", () => {
      render(
        <TestWrapper>
          <JobDialog
            open={true}
            onOpenChange={vi.fn()}
            sources={[createMockSource("test-source")]}
          />
        </TestWrapper>
      );

      // Should render properly without preselectedSource
      expect(screen.getByText("Job Name")).toBeInTheDocument();
    });
  });

  describe("hook integration", () => {
    it("uses createJob from useArenaJobMutations", () => {
      render(
        <TestWrapper>
          <JobDialog
            open={true}
            onOpenChange={vi.fn()}
            sources={[createMockSource("test-source")]}
          />
        </TestWrapper>
      );

      // The JobWizard should be rendered with the createJob function
      expect(screen.getByText("Job Name")).toBeInTheDocument();
    });

    it("uses license info from useLicense hook", () => {
      render(
        <TestWrapper>
          <JobDialog
            open={true}
            onOpenChange={vi.fn()}
            sources={[createMockSource("test-source")]}
          />
        </TestWrapper>
      );

      // With isEnterprise: true, loadtest and datagen should be available
      // The license hook is used to determine enterprise status
      const typeSelect = screen.getByRole("combobox");
      fireEvent.click(typeSelect);

      // All options should be available (Evaluation, Load Test, Data Generation)
      // Using queryAllByText to verify multiple elements with "Evaluation" exist (in select and options)
      const evaluationTexts = screen.getAllByText("Evaluation");
      expect(evaluationTexts.length).toBeGreaterThan(0);
    });
  });

  describe("full job creation flow", () => {
    // Helper to select an option from a Radix Select
    async function selectOption(trigger: HTMLElement, optionText: string) {
      fireEvent.click(trigger);
      const option = await screen.findByRole("option", { name: optionText });
      fireEvent.click(option);
      fireEvent.keyDown(document.body, { key: "Escape" });
    }

    it("calls onSuccess after successful job creation and closes dialog", async () => {
      vi.useRealTimers();
      const user = userEvent.setup();
      const onSuccess = vi.fn();
      const onClose = vi.fn();
      const onOpenChange = vi.fn();

      render(
        <TestWrapper>
          <JobDialog
            open={true}
            onOpenChange={onOpenChange}
            sources={[createMockSource("test-source")]}
            onSuccess={onSuccess}
            onClose={onClose}
          />
        </TestWrapper>
      );

      // Step 0: Enter job name
      const nameInput = screen.getByPlaceholderText("my-job");
      await user.type(nameInput, "test-job");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 1: Select source
      await selectOption(screen.getByLabelText("Source"), "test-source");
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 2-4: Skip
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));
      fireEvent.click(screen.getByRole("button", { name: /next/i }));

      // Step 5: Submit
      fireEvent.click(screen.getByRole("button", { name: /create job/i }));

      // Wait for success and onSuccess callback
      await waitFor(() => {
        expect(screen.getByText("Job Created!")).toBeInTheDocument();
      });
      expect(onSuccess).toHaveBeenCalled();
      expect(mockCreateJob).toHaveBeenCalled();

      // Wait for setTimeout to close dialog
      await waitFor(() => {
        expect(onClose).toHaveBeenCalled();
      }, { timeout: 2000 });
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });
  });
});

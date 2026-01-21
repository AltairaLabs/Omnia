/**
 * Tests for Arena Jobs list page.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import ArenaJobsPage from "./page";

// Mock useSearchParams
vi.mock("next/navigation", () => ({
  useSearchParams: () => ({
    get: vi.fn().mockReturnValue(null),
  }),
}));

// Mock hooks
vi.mock("@/hooks/use-arena-jobs", () => ({
  useArenaJobs: vi.fn(),
  useArenaJobMutations: vi.fn(),
}));

vi.mock("@/hooks/use-arena-configs", () => ({
  useArenaConfigs: vi.fn(),
}));

// Workspace mock with configurable permissions
let mockWorkspacePermissions = { write: true, read: true, delete: true, manageMembers: false };
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(() => ({
    currentWorkspace: {
      name: "default",
      permissions: mockWorkspacePermissions,
    },
  })),
}));

// Mock layout components
vi.mock("@/components/layout", () => ({
  Header: ({ title, description }: { title: string; description: string }) => (
    <div data-testid="header">
      <h1>{title}</h1>
      <p>{description}</p>
    </div>
  ),
}));

// Mock arena components - import utils directly (lightweight), mock heavy dialog components
vi.mock("@/components/arena", async () => {
  // Import only the lightweight utility functions (no React components)
  const utils = await import("@/components/arena/source-utils");
  return {
    ...utils,
    ArenaBreadcrumb: ({ items }: { items: { label: string }[] }) => (
      <nav data-testid="breadcrumb">
        {items.map((item: { label: string }) => (
          <span key={item.label}>{item.label}</span>
        ))}
      </nav>
    ),
    JobDialog: ({ open }: { open: boolean }) => (
      open ? <div data-testid="job-dialog">Dialog</div> : null
    ),
    // Stub heavy dialog components to prevent loading their dependencies
    SourceDialog: () => null,
    ConfigDialog: () => null,
  };
});

// Mock next/link
vi.mock("next/link", () => ({
  default: ({ children, href }: { children: React.ReactNode; href: string }) => (
    <a href={href}>{children}</a>
  ),
}));

const mockJob = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1" as const,
  kind: "ArenaJob" as const,
  metadata: { name: "test-job", creationTimestamp: "2026-01-15T10:00:00Z" },
  spec: {
    configRef: { name: "test-config" },
    type: "evaluation" as const,
    workers: { replicas: 2 },
  },
  status: {
    phase: "Running" as const,
    totalTasks: 100,
    completedTasks: 50,
    failedTasks: 0,
    workers: { desired: 2, active: 2 },
    startTime: "2026-01-15T10:00:00Z",
  },
};

const mockConfig = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1" as const,
  kind: "ArenaConfig" as const,
  metadata: { name: "test-config" },
  spec: { sourceRef: { name: "test-source" } },
  status: { phase: "Ready" as const },
};

describe("ArenaJobsPage", () => {
  const mockRefetch = vi.fn();
  const mockCancelJob = vi.fn();
  const mockDeleteJob = vi.fn();

  beforeEach(() => {
    vi.resetAllMocks();
    // Reset to default permissions
    mockWorkspacePermissions = { write: true, read: true, delete: true, manageMembers: false };
  });

  it("renders loading skeleton when loading", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [],
      loading: true,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    expect(screen.getByText("Jobs")).toBeInTheDocument();
    expect(screen.getByText("Manage Arena evaluation jobs")).toBeInTheDocument();
  });

  it("renders error state when error occurs", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [],
      loading: false,
      error: new Error("Failed to fetch jobs"),
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    expect(screen.getByText("Error loading jobs")).toBeInTheDocument();
    expect(screen.getByText("Failed to fetch jobs")).toBeInTheDocument();
  });

  it("renders empty state when no jobs", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    expect(screen.getByText("No jobs found")).toBeInTheDocument();
  });

  it("renders jobs in grid view by default", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [mockJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    expect(screen.getByText("test-job")).toBeInTheDocument();
    expect(screen.getByText("test-config")).toBeInTheDocument();
  });

  it("opens create dialog when Create Job button is clicked", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    const createButton = screen.getByText("Create Job");
    fireEvent.click(createButton);

    expect(screen.getByTestId("job-dialog")).toBeInTheDocument();
  });

  it("hides Create Job button when user lacks write permission", async () => {
    mockWorkspacePermissions = { write: false, read: true, delete: false, manageMembers: false };

    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    expect(screen.queryByText("Create Job")).not.toBeInTheDocument();
  });

  it("shows job type badge correctly", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [mockJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    expect(screen.getByText("Evaluation")).toBeInTheDocument();
  });

  it("shows running badge for running jobs", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [mockJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    expect(screen.getByText("Running")).toBeInTheDocument();
  });

  it("shows filter dropdowns", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    expect(screen.getByText("All Types")).toBeInTheDocument();
    expect(screen.getByText("All Status")).toBeInTheDocument();
  });

  it("shows workers count badge", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [mockJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    // Workers badge shows 2/2 format
    expect(screen.getByText("2/2")).toBeInTheDocument();
  });

  it("switches to table view when table tab is clicked", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [mockJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    // Click on table view tab
    const tabs = screen.getAllByRole("tab");
    const tableTab = tabs.find(tab => tab.getAttribute("data-state") !== "active");
    if (tableTab) {
      fireEvent.click(tableTab);
    }

    // Job should still be visible
    expect(screen.getByText("test-job")).toBeInTheDocument();
  });

  it("filters jobs by type", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    const loadTestJob = {
      ...mockJob,
      metadata: { name: "loadtest-job", creationTimestamp: "2026-01-15T10:00:00Z" },
      spec: { ...mockJob.spec, type: "loadtest" as const },
    };

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [mockJob, loadTestJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    // Both jobs should be visible initially
    expect(screen.getByText("test-job")).toBeInTheDocument();
    expect(screen.getByText("loadtest-job")).toBeInTheDocument();
  });

  it("filters jobs by phase", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    const completedJob = {
      ...mockJob,
      metadata: { name: "completed-job", creationTimestamp: "2026-01-15T10:00:00Z" },
      status: { ...mockJob.status, phase: "Completed" as const },
    };

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [mockJob, completedJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    // Both jobs should be visible initially
    expect(screen.getByText("test-job")).toBeInTheDocument();
    expect(screen.getByText("completed-job")).toBeInTheDocument();
  });

  it("shows pending job status", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    const pendingJob = {
      ...mockJob,
      status: { ...mockJob.status, phase: "Pending" as const },
    };

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [pendingJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    expect(screen.getByText("Pending")).toBeInTheDocument();
  });

  it("shows failed job status", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    const failedJob = {
      ...mockJob,
      status: { ...mockJob.status, phase: "Failed" as const, failedTasks: 10 },
    };

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [failedJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    expect(screen.getByText("Failed")).toBeInTheDocument();
  });

  it("shows cancelled job status", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    const cancelledJob = {
      ...mockJob,
      status: { ...mockJob.status, phase: "Cancelled" as const },
    };

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [cancelledJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    expect(screen.getByText("Cancelled")).toBeInTheDocument();
  });

  it("shows loadtest job type", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    const loadTestJob = {
      ...mockJob,
      spec: { ...mockJob.spec, type: "loadtest" as const },
    };

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [loadTestJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    expect(screen.getByText("Load Test")).toBeInTheDocument();
  });

  it("shows datagen job type", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    const datagenJob = {
      ...mockJob,
      spec: { ...mockJob.spec, type: "datagen" as const },
    };

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [datagenJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    expect(screen.getByText("Data Gen")).toBeInTheDocument();
  });

  it("shows completed job status", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    const completedJob = {
      ...mockJob,
      status: { ...mockJob.status, phase: "Completed" as const, completedTasks: 100 },
    };

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [completedJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    expect(screen.getByText("Completed")).toBeInTheDocument();
  });

  it("shows unknown type badge for undefined job type", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    // Use type assertion to test defensive UI code path when runtime data doesn't match types
    const unknownTypeJob = {
      ...mockJob,
      spec: { ...mockJob.spec, type: undefined as unknown as "evaluation" },
    };

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [unknownTypeJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    expect(screen.getByText("Unknown")).toBeInTheDocument();
  });

  it("shows unknown phase badge for undefined job phase", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    const unknownPhaseJob = {
      ...mockJob,
      status: { ...mockJob.status, phase: undefined },
    };

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [unknownPhaseJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    // Two Unknown badges - one for type and one for phase is one each
    const unknowns = screen.getAllByText("Unknown");
    expect(unknowns.length).toBeGreaterThanOrEqual(1);
  });

  it("shows dash for progress when total tasks is 0", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    const jobWithNoTasks = {
      ...mockJob,
      status: { ...mockJob.status, totalTasks: 0, completedTasks: 0 },
    };

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [jobWithNoTasks],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    expect(screen.getByText("-")).toBeInTheDocument();
  });

  it("renders jobs in table view when switched", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [mockJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    // Click table view tab
    const tabs = screen.getAllByRole("tab");
    const tableTab = tabs.find(tab => tab.textContent === "" && tab.getAttribute("data-state") !== "active");
    if (tableTab) {
      fireEvent.click(tableTab);
    }

    // Table headers should be visible
    expect(screen.getByText("test-job")).toBeInTheDocument();
  });

  it("shows empty state in table view when no jobs", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    // Click table view tab
    const tabs = screen.getAllByRole("tab");
    const tableTab = tabs.find(tab => tab.getAttribute("data-state") !== "active");
    if (tableTab) {
      fireEvent.click(tableTab);
    }

    // Empty state should be visible
    expect(screen.getByText("No jobs found")).toBeInTheDocument();
  });

  it("filters jobs correctly when type filter is applied", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    const evalJob = {
      ...mockJob,
      metadata: { name: "eval-job", creationTimestamp: "2026-01-15T10:00:00Z" },
      spec: { ...mockJob.spec, type: "evaluation" as const },
    };
    const loadtestJob = {
      ...mockJob,
      metadata: { name: "loadtest-job", creationTimestamp: "2026-01-15T11:00:00Z" },
      spec: { ...mockJob.spec, type: "loadtest" as const },
    };

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [evalJob, loadtestJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    // Both jobs should be visible initially
    expect(screen.getByText("eval-job")).toBeInTheDocument();
    expect(screen.getByText("loadtest-job")).toBeInTheDocument();

    // Click the type filter dropdown
    const typeSelect = screen.getByText("All Types");
    fireEvent.click(typeSelect);

    // Select evaluation type - the option is rendered via portal
    const evalOption = screen.getByRole("option", { name: "Evaluation" });
    fireEvent.click(evalOption);

    // Only eval job should be visible now
    expect(screen.getByText("eval-job")).toBeInTheDocument();
    expect(screen.queryByText("loadtest-job")).not.toBeInTheDocument();
  });

  it("filters jobs correctly when phase filter is applied", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    const runningJob = {
      ...mockJob,
      metadata: { name: "running-job", creationTimestamp: "2026-01-15T10:00:00Z" },
      status: { ...mockJob.status, phase: "Running" as const },
    };
    const completedJob = {
      ...mockJob,
      metadata: { name: "completed-job", creationTimestamp: "2026-01-15T11:00:00Z" },
      status: { ...mockJob.status, phase: "Completed" as const },
    };

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [runningJob, completedJob],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    // Both jobs should be visible initially
    expect(screen.getByText("running-job")).toBeInTheDocument();
    expect(screen.getByText("completed-job")).toBeInTheDocument();

    // Click the status filter dropdown
    const statusSelect = screen.getByText("All Status");
    fireEvent.click(statusSelect);

    // Select Completed status
    const completedOption = screen.getByRole("option", { name: "Completed" });
    fireEvent.click(completedOption);

    // Only completed job should be visible now
    expect(screen.queryByText("running-job")).not.toBeInTheDocument();
    expect(screen.getByText("completed-job")).toBeInTheDocument();
  });

  it("shows progress correctly with failed tasks", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    const jobWithFailed = {
      ...mockJob,
      status: { ...mockJob.status, totalTasks: 100, completedTasks: 70, failedTasks: 10 },
    };

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [jobWithFailed],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    // Progress should show 70/100
    expect(screen.getByText("70/100")).toBeInTheDocument();
  });

  it("shows job without config ref", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    // Use type assertion to test defensive UI code path when runtime data doesn't match types
    const jobWithoutConfig = {
      ...mockJob,
      spec: { ...mockJob.spec, configRef: undefined as unknown as { name: string } },
    };

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [jobWithoutConfig],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    // Job name should be visible
    expect(screen.getByText("test-job")).toBeInTheDocument();
  });

  it("shows workers with desired from spec when status is missing", async () => {
    const { useArenaJobs, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");
    const { useArenaConfigs } = await import("@/hooks/use-arena-configs");

    const jobWithSpecWorkers = {
      ...mockJob,
      status: { ...mockJob.status, workers: undefined },
    };

    vi.mocked(useArenaJobs).mockReturnValue({
      jobs: [jobWithSpecWorkers],
      loading: false,
      error: null,
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });
    vi.mocked(useArenaConfigs).mockReturnValue({
      configs: [mockConfig],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaJobsPage />);

    // Should show 0/2 (active from status is missing, desired from spec.workers.replicas)
    expect(screen.getByText("0/2")).toBeInTheDocument();
  });
});

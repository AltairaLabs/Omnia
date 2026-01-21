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

// Mock arena components
vi.mock("@/components/arena", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/components/arena")>();
  return {
    ...actual,
    ArenaBreadcrumb: ({ items }: { items: { label: string }[] }) => (
      <nav data-testid="breadcrumb">
        {items.map((item) => (
          <span key={item.label}>{item.label}</span>
        ))}
      </nav>
    ),
    JobDialog: ({ open }: { open: boolean }) => (
      open ? <div data-testid="job-dialog">Dialog</div> : null
    ),
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
});

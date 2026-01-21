/**
 * Tests for Arena Job detail page.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import ArenaJobDetailPage from "./page";

// Mock useParams and useRouter
const mockPush = vi.fn();
vi.mock("next/navigation", () => ({
  useParams: () => ({
    name: "test-job",
  }),
  useRouter: () => ({
    push: mockPush,
    back: vi.fn(),
    forward: vi.fn(),
    refresh: vi.fn(),
    replace: vi.fn(),
    prefetch: vi.fn(),
  }),
}));

// Mock hooks
vi.mock("@/hooks/use-arena-jobs", () => ({
  useArenaJob: vi.fn(),
  useArenaJobMutations: vi.fn(),
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
    // Stub heavy dialog components to prevent loading their dependencies
    SourceDialog: () => null,
    ConfigDialog: () => null,
    JobDialog: () => null,
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
    timeout: "30m",
    evaluation: {
      passingThreshold: 0.8,
      continueOnFailure: true,
      outputFormats: ["json", "junit"] as ("json" | "junit" | "csv")[],
    },
  },
  status: {
    phase: "Running" as const,
    totalTasks: 100,
    completedTasks: 50,
    failedTasks: 0,
    workers: { desired: 2, active: 2 },
    startTime: "2026-01-15T10:00:00Z",
    conditions: [
      {
        type: "Ready",
        status: "True" as const,
        reason: "JobRunning",
        message: "Job is running",
        lastTransitionTime: "2026-01-15T10:00:00Z",
      },
    ],
  },
};

describe("ArenaJobDetailPage", () => {
  const mockRefetch = vi.fn();
  const mockCancelJob = vi.fn();
  const mockDeleteJob = vi.fn();

  beforeEach(() => {
    vi.resetAllMocks();
    // Reset to default permissions
    mockWorkspacePermissions = { write: true, read: true, delete: true, manageMembers: false };
  });

  it("renders loading skeleton when loading", async () => {
    const { useArenaJob, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJob).mockReturnValue({
      job: null,
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

    render(<ArenaJobDetailPage />);

    expect(screen.getByText("Job Details")).toBeInTheDocument();
  });

  it("renders error state when error occurs", async () => {
    const { useArenaJob, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJob).mockReturnValue({
      job: null,
      loading: false,
      error: new Error("Job not found"),
      refetch: mockRefetch,
    });
    vi.mocked(useArenaJobMutations).mockReturnValue({
      createJob: vi.fn(),
      cancelJob: mockCancelJob,
      deleteJob: mockDeleteJob,
      loading: false,
      error: null,
    });

    render(<ArenaJobDetailPage />);

    expect(screen.getAllByText("Error loading job").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Job not found").length).toBeGreaterThan(0);
  });

  it("renders not found when job does not exist", async () => {
    const { useArenaJob, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJob).mockReturnValue({
      job: null,
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

    render(<ArenaJobDetailPage />);

    expect(screen.getAllByText("Job not found").length).toBeGreaterThan(0);
  });

  it("displays job details in overview tab", async () => {
    const { useArenaJob, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJob).mockReturnValue({
      job: mockJob,
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

    render(<ArenaJobDetailPage />);

    expect(screen.getAllByText("test-job").length).toBeGreaterThan(0);
    expect(screen.getByText("Progress")).toBeInTheDocument();
    expect(screen.getByText("Total Tasks")).toBeInTheDocument();
  });

  it("shows cancel button for running job with write permission", async () => {
    const { useArenaJob, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJob).mockReturnValue({
      job: mockJob,
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

    render(<ArenaJobDetailPage />);

    expect(screen.getByText("Cancel")).toBeInTheDocument();
  });

  it("shows delete button for completed job", async () => {
    const { useArenaJob, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    const completedJob = {
      ...mockJob,
      status: { ...mockJob.status, phase: "Completed" as const },
    };

    vi.mocked(useArenaJob).mockReturnValue({
      job: completedJob,
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

    render(<ArenaJobDetailPage />);

    expect(screen.getByText("Delete")).toBeInTheDocument();
  });

  it("shows refresh button", async () => {
    const { useArenaJob, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJob).mockReturnValue({
      job: mockJob,
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

    render(<ArenaJobDetailPage />);

    expect(screen.getByText("Refresh")).toBeInTheDocument();
  });

  it("displays job type badge", async () => {
    const { useArenaJob, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJob).mockReturnValue({
      job: mockJob,
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

    render(<ArenaJobDetailPage />);

    expect(screen.getAllByText("Evaluation").length).toBeGreaterThan(0);
  });

  it("displays workers status card", async () => {
    const { useArenaJob, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJob).mockReturnValue({
      job: mockJob,
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

    render(<ArenaJobDetailPage />);

    expect(screen.getByText("Workers")).toBeInTheDocument();
    expect(screen.getByText("Desired")).toBeInTheDocument();
    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  it("shows evaluation settings for evaluation job", async () => {
    const { useArenaJob, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJob).mockReturnValue({
      job: mockJob,
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

    render(<ArenaJobDetailPage />);

    expect(screen.getByText("Evaluation Settings")).toBeInTheDocument();
    expect(screen.getByText("Passing Threshold")).toBeInTheDocument();
  });

  it("hides cancel button for read-only user", async () => {
    mockWorkspacePermissions = { write: false, read: true, delete: false, manageMembers: false };

    const { useArenaJob, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJob).mockReturnValue({
      job: mockJob,
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

    render(<ArenaJobDetailPage />);

    expect(screen.queryByText("Cancel")).not.toBeInTheDocument();
  });

  it("shows timing information", async () => {
    const { useArenaJob, useArenaJobMutations } = await import("@/hooks/use-arena-jobs");

    vi.mocked(useArenaJob).mockReturnValue({
      job: mockJob,
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

    render(<ArenaJobDetailPage />);

    expect(screen.getByText("Timing")).toBeInTheDocument();
    expect(screen.getByText("Started")).toBeInTheDocument();
    expect(screen.getByText("Duration")).toBeInTheDocument();
    expect(screen.getByText("Timeout")).toBeInTheDocument();
  });
});

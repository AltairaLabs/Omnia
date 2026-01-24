/**
 * Tests for Arena overview page.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import ArenaPage from "./page";

// Mock useArenaStats hook
vi.mock("@/hooks", () => ({
  useArenaStats: vi.fn(),
}));

// Mock layout components that require providers
vi.mock("@/components/layout", () => ({
  Header: ({ title, description }: { title: string; description: string }) => (
    <div data-testid="header">
      <h1>{title}</h1>
      <p>{description}</p>
    </div>
  ),
}));

// Mock next/link
vi.mock("next/link", () => ({
  default: ({ children, href }: { children: React.ReactNode; href: string }) => (
    <a href={href}>{children}</a>
  ),
}));

describe("ArenaPage", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  it("renders loading skeleton when loading", async () => {
    const { useArenaStats } = await import("@/hooks");
    vi.mocked(useArenaStats).mockReturnValue({
      stats: null,
      recentJobs: [],
      loading: true,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaPage />);

    // Should show header
    expect(screen.getByText("Arena")).toBeInTheDocument();
    expect(screen.getByText("Evaluate, load test, and generate data for your AI agents")).toBeInTheDocument();
  });

  it("renders error state when error occurs", async () => {
    const { useArenaStats } = await import("@/hooks");
    vi.mocked(useArenaStats).mockReturnValue({
      stats: null,
      recentJobs: [],
      loading: false,
      error: new Error("Failed to fetch stats"),
      refetch: vi.fn(),
    });

    render(<ArenaPage />);

    expect(screen.getByText("Error loading Arena stats")).toBeInTheDocument();
    expect(screen.getByText("Failed to fetch stats")).toBeInTheDocument();
  });

  it("renders stats cards with data", async () => {
    const { useArenaStats } = await import("@/hooks");
    vi.mocked(useArenaStats).mockReturnValue({
      stats: {
        sources: { total: 5, ready: 3, failed: 1, active: 3 },
        jobs: { total: 20, running: 2, queued: 3, completed: 15, failed: 2, successRate: 0.882 },
      },
      recentJobs: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaPage />);

    // Check stat cards
    expect(screen.getByText("Active Sources")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument(); // active sources
    expect(screen.getByText("1 failed")).toBeInTheDocument();

    expect(screen.getByText("Running Jobs")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument(); // running jobs
    expect(screen.getByText("3 queued")).toBeInTheDocument();

    expect(screen.getByText("Success Rate")).toBeInTheDocument();
    expect(screen.getByText("88%")).toBeInTheDocument(); // 0.882 * 100 rounded
    expect(screen.getByText("15 completed")).toBeInTheDocument();
  });

  it("renders empty jobs message when no jobs", async () => {
    const { useArenaStats } = await import("@/hooks");
    vi.mocked(useArenaStats).mockReturnValue({
      stats: {
        sources: { total: 0, ready: 0, failed: 0, active: 0 },
        jobs: { total: 0, running: 0, queued: 0, completed: 0, failed: 0, successRate: 0 },
      },
      recentJobs: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaPage />);

    expect(screen.getByText("No jobs found. Create your first job to get started.")).toBeInTheDocument();
  });

  it("renders recent jobs table with jobs", async () => {
    const { useArenaStats } = await import("@/hooks");
    vi.mocked(useArenaStats).mockReturnValue({
      stats: {
        sources: { total: 1, ready: 1, failed: 0, active: 1 },
        jobs: { total: 2, running: 1, queued: 0, completed: 1, failed: 0, successRate: 1 },
      },
      recentJobs: [
        {
          metadata: { name: "eval-job-1", creationTimestamp: "2026-01-20T10:00:00Z" },
          spec: { type: "evaluation", configRef: { name: "config-1" } },
          status: { phase: "Running", completedTasks: 5, totalTasks: 10 },
        },
        {
          metadata: { name: "load-job-1", creationTimestamp: "2026-01-20T09:00:00Z" },
          spec: { type: "loadtest", configRef: { name: "config-2" } },
          status: { phase: "Succeeded", completedTasks: 100, totalTasks: 100 },
        },
      ] as any,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaPage />);

    // Check table headers
    expect(screen.getByText("Name")).toBeInTheDocument();
    expect(screen.getByText("Type")).toBeInTheDocument();
    expect(screen.getByText("Status")).toBeInTheDocument();
    expect(screen.getByText("Progress")).toBeInTheDocument();
    expect(screen.getByText("Created")).toBeInTheDocument();

    // Check job names
    expect(screen.getByText("eval-job-1")).toBeInTheDocument();
    expect(screen.getByText("load-job-1")).toBeInTheDocument();

    // Check job types
    expect(screen.getByText("Evaluation")).toBeInTheDocument();
    expect(screen.getByText("Load Test")).toBeInTheDocument();

    // Check job statuses
    expect(screen.getByText("Running")).toBeInTheDocument();
    expect(screen.getByText("Succeeded")).toBeInTheDocument();

    // Check progress percentages in table (use getAllByText since success rate card also shows %)
    const progressCells = screen.getAllByText(/^\d+%$/);
    expect(progressCells.length).toBeGreaterThanOrEqual(2); // At least 50% and 100% from table
  });

  it("renders all job status badges correctly", async () => {
    const { useArenaStats } = await import("@/hooks");
    vi.mocked(useArenaStats).mockReturnValue({
      stats: {
        sources: { total: 0, ready: 0, failed: 0, active: 0 },
        jobs: { total: 5, running: 1, queued: 1, completed: 1, failed: 1, successRate: 0.5 },
      },
      recentJobs: [
        { metadata: { name: "job-1" }, spec: { type: "evaluation", configRef: { name: "c1" } }, status: { phase: "Running" } },
        { metadata: { name: "job-2" }, spec: { type: "evaluation", configRef: { name: "c2" } }, status: { phase: "Succeeded" } },
        { metadata: { name: "job-3" }, spec: { type: "evaluation", configRef: { name: "c3" } }, status: { phase: "Failed" } },
        { metadata: { name: "job-4" }, spec: { type: "evaluation", configRef: { name: "c4" } }, status: { phase: "Cancelled" } },
        { metadata: { name: "job-5" }, spec: { type: "evaluation", configRef: { name: "c5" } }, status: { phase: "Pending" } },
      ] as any,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaPage />);

    expect(screen.getByText("Running")).toBeInTheDocument();
    expect(screen.getByText("Succeeded")).toBeInTheDocument();
    expect(screen.getByText("Failed")).toBeInTheDocument();
    expect(screen.getByText("Cancelled")).toBeInTheDocument();
    expect(screen.getByText("Pending")).toBeInTheDocument();
  });

  it("renders all job type badges correctly", async () => {
    const { useArenaStats } = await import("@/hooks");
    vi.mocked(useArenaStats).mockReturnValue({
      stats: {
        sources: { total: 0, ready: 0, failed: 0, active: 0 },
        jobs: { total: 3, running: 3, queued: 0, completed: 0, failed: 0, successRate: 0 },
      },
      recentJobs: [
        { metadata: { name: "job-1" }, spec: { type: "evaluation", configRef: { name: "c1" } }, status: { phase: "Running" } },
        { metadata: { name: "job-2" }, spec: { type: "loadtest", configRef: { name: "c2" } }, status: { phase: "Running" } },
        { metadata: { name: "job-3" }, spec: { type: "datagen", configRef: { name: "c3" } }, status: { phase: "Running" } },
      ] as any,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaPage />);

    expect(screen.getByText("Evaluation")).toBeInTheDocument();
    expect(screen.getByText("Load Test")).toBeInTheDocument();
    expect(screen.getByText("Data Gen")).toBeInTheDocument();
  });

  it("renders unknown status and type badges", async () => {
    const { useArenaStats } = await import("@/hooks");
    vi.mocked(useArenaStats).mockReturnValue({
      stats: {
        sources: { total: 0, ready: 0, failed: 0, active: 0 },
        jobs: { total: 1, running: 0, queued: 0, completed: 0, failed: 0, successRate: 0 },
      },
      recentJobs: [
        { metadata: { name: "job-1" }, spec: { type: "custom-type", configRef: { name: "c1" } }, status: { phase: "CustomPhase" } },
      ] as any,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaPage />);

    expect(screen.getByText("custom-type")).toBeInTheDocument();
    expect(screen.getByText("CustomPhase")).toBeInTheDocument();
  });

  it("handles missing metadata gracefully", async () => {
    const { useArenaStats } = await import("@/hooks");
    vi.mocked(useArenaStats).mockReturnValue({
      stats: {
        sources: { total: 0, ready: 0, failed: 0, active: 0 },
        jobs: { total: 1, running: 0, queued: 0, completed: 0, failed: 0, successRate: 0 },
      },
      recentJobs: [
        { spec: { type: undefined, configRef: { name: "c1" } }, status: { phase: undefined } },
      ] as any,
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaPage />);

    // Should render with fallback values - use getAllByText since there might be multiple Unknown badges
    const unknownBadges = screen.getAllByText("Unknown");
    expect(unknownBadges.length).toBeGreaterThanOrEqual(1); // At least one Unknown badge (type or status)
    expect(screen.getByText("0%")).toBeInTheDocument(); // 0 progress
    expect(screen.getByText("-")).toBeInTheDocument(); // No date
  });

  it("renders quick links section", async () => {
    const { useArenaStats } = await import("@/hooks");
    vi.mocked(useArenaStats).mockReturnValue({
      stats: {
        sources: { total: 0, ready: 0, failed: 0, active: 0 },
        jobs: { total: 0, running: 0, queued: 0, completed: 0, failed: 0, successRate: 0 },
      },
      recentJobs: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaPage />);

    expect(screen.getByText("Manage Sources")).toBeInTheDocument();
    expect(screen.getByText("Run Jobs")).toBeInTheDocument();
  });

  it("shows N/A for success rate when no jobs completed", async () => {
    const { useArenaStats } = await import("@/hooks");
    vi.mocked(useArenaStats).mockReturnValue({
      stats: {
        sources: { total: 0, ready: 0, failed: 0, active: 0 },
        jobs: { total: 0, running: 0, queued: 0, completed: 0, failed: 0, successRate: 0 },
      },
      recentJobs: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaPage />);

    expect(screen.getByText("N/A")).toBeInTheDocument();
  });

  it("renders correct links in stat cards", async () => {
    const { useArenaStats } = await import("@/hooks");
    vi.mocked(useArenaStats).mockReturnValue({
      stats: {
        sources: { total: 1, ready: 1, failed: 0, active: 1 },
        jobs: { total: 1, running: 1, queued: 0, completed: 0, failed: 0, successRate: 0 },
      },
      recentJobs: [],
      loading: false,
      error: null,
      refetch: vi.fn(),
    });

    render(<ArenaPage />);

    // Check links exist
    const links = screen.getAllByRole("link");
    const hrefs = links.map(link => link.getAttribute("href"));

    expect(hrefs).toContain("/arena/sources");
    expect(hrefs).toContain("/arena/jobs");
  });
});

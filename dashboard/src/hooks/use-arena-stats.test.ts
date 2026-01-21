/**
 * Tests for useArenaStats hook.
 *
 * These hooks use the DataService abstraction (via useDataService)
 * to support both demo mode and live mode.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useArenaStats } from "./use-arena-stats";

// Mock workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(),
}));

// Mock data service
const mockGetArenaStats = vi.fn();
const mockGetArenaJobs = vi.fn();

vi.mock("@/lib/data/provider", () => ({
  useDataService: () => ({
    getArenaStats: mockGetArenaStats,
    getArenaJobs: mockGetArenaJobs,
  }),
}));

const mockWorkspace = {
  name: "test-workspace",
  displayName: "Test",
  environment: "development" as const,
  namespace: "test-ns",
  role: "viewer" as const,
  permissions: { read: true, write: false, delete: false, manageMembers: false },
};

describe("useArenaStats", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns empty stats when no workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaStats());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.stats).toEqual({
      sources: { total: 0, ready: 0, failed: 0, active: 0 },
      configs: { total: 0, ready: 0, scenarios: 0 },
      jobs: { total: 0, running: 0, queued: 0, completed: 0, failed: 0, successRate: 0 },
    });
    expect(result.current.recentJobs).toEqual([]);
    expect(result.current.error).toBeNull();
    expect(mockGetArenaStats).not.toHaveBeenCalled();
  });

  it("fetches stats and jobs when workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const mockStats = {
      sources: { total: 3, ready: 2, failed: 1, active: 2 },
      configs: { total: 5, ready: 4, scenarios: 15 },
      jobs: { total: 10, running: 2, queued: 1, completed: 6, failed: 1, successRate: 0.857 },
    };

    const mockJobs = [
      {
        metadata: { name: "job-1", creationTimestamp: "2026-01-20T10:00:00Z" },
        spec: { type: "evaluation" },
        status: { phase: "Running", completedTasks: 5, totalTasks: 10 },
      },
      {
        metadata: { name: "job-2", creationTimestamp: "2026-01-20T09:00:00Z" },
        spec: { type: "load-test" },
        status: { phase: "Completed", completedTasks: 100, totalTasks: 100 },
      },
    ];

    mockGetArenaStats.mockResolvedValueOnce(mockStats);
    mockGetArenaJobs.mockResolvedValueOnce(mockJobs);

    const { result } = renderHook(() => useArenaStats());

    expect(result.current.loading).toBe(true);

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.stats).toEqual(mockStats);
    expect(result.current.recentJobs).toHaveLength(2);
    expect(result.current.error).toBeNull();

    expect(mockGetArenaStats).toHaveBeenCalledWith("test-workspace");
    expect(mockGetArenaJobs).toHaveBeenCalledWith("test-workspace", { sort: "recent", limit: 5 });
  });

  it("handles stats fetch error", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockGetArenaStats.mockRejectedValueOnce(new Error("Failed to fetch stats"));
    mockGetArenaJobs.mockResolvedValueOnce([]);

    const { result } = renderHook(() => useArenaStats());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.stats).toBeNull();
    expect(result.current.recentJobs).toEqual([]);
    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.error?.message).toContain("Failed to fetch stats");
  });

  it("refetch function triggers new fetch", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const mockStats = {
      sources: { total: 1, ready: 1, failed: 0, active: 1 },
      configs: { total: 1, ready: 1, scenarios: 5 },
      jobs: { total: 0, running: 0, queued: 0, completed: 0, failed: 0, successRate: 0 },
    };

    mockGetArenaStats.mockResolvedValue(mockStats);
    mockGetArenaJobs.mockResolvedValue([]);

    const { result } = renderHook(() => useArenaStats());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockGetArenaStats).toHaveBeenCalledTimes(1);
    expect(mockGetArenaJobs).toHaveBeenCalledTimes(1);

    // Call refetch
    result.current.refetch();

    await waitFor(() => {
      expect(mockGetArenaStats).toHaveBeenCalledTimes(2);
      expect(mockGetArenaJobs).toHaveBeenCalledTimes(2);
    });
  });
});

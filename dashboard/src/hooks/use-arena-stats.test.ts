/**
 * Tests for useArenaStats hook.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useArenaStats } from "./use-arena-stats";

// Mock workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(),
}));

// Mock fetch
const mockFetch = vi.fn();
global.fetch = mockFetch;

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
      jobs: { total: 0, running: 0, queued: 0, completed: 0, failed: 0, successRate: 0 },
    });
    expect(result.current.recentJobs).toEqual([]);
    expect(result.current.error).toBeNull();
  });

  it("fetches stats and jobs when workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: {
        name: "test-workspace",
        displayName: "Test",
        environment: "development" as const,
        namespace: "test-ns",
        role: "viewer" as const,
        permissions: { read: true, write: false, delete: false, manageMembers: false },
      },
      setCurrentWorkspace: vi.fn(),
      workspaces: [{
        name: "test-workspace",
        displayName: "Test",
        environment: "development" as const,
        namespace: "test-ns",
        role: "viewer" as const,
        permissions: { read: true, write: false, delete: false, manageMembers: false },
      }],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const mockStats = {
      sources: { total: 3, ready: 2, failed: 1, active: 2 },
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

    mockFetch
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockStats),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockJobs),
      });

    const { result } = renderHook(() => useArenaStats());

    expect(result.current.loading).toBe(true);

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.stats).toEqual(mockStats);
    expect(result.current.recentJobs).toHaveLength(2);
    expect(result.current.recentJobs[0].metadata?.name).toBe("job-1"); // Most recent first
    expect(result.current.error).toBeNull();

    expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-workspace/arena/stats");
    expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-workspace/arena/jobs?limit=5");
  });

  it("handles stats fetch error", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: {
        name: "test-workspace",
        displayName: "Test",
        environment: "development" as const,
        namespace: "test-ns",
        role: "viewer" as const,
        permissions: { read: true, write: false, delete: false, manageMembers: false },
      },
      setCurrentWorkspace: vi.fn(),
      workspaces: [{
        name: "test-workspace",
        displayName: "Test",
        environment: "development" as const,
        namespace: "test-ns",
        role: "viewer" as const,
        permissions: { read: true, write: false, delete: false, manageMembers: false },
      }],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      statusText: "Internal Server Error",
    });

    const { result } = renderHook(() => useArenaStats());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.stats).toBeNull();
    expect(result.current.recentJobs).toEqual([]);
    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.error?.message).toContain("Failed to fetch stats");
  });

  it("handles jobs fetch error gracefully (stats still work)", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: {
        name: "test-workspace",
        displayName: "Test",
        environment: "development" as const,
        namespace: "test-ns",
        role: "viewer" as const,
        permissions: { read: true, write: false, delete: false, manageMembers: false },
      },
      setCurrentWorkspace: vi.fn(),
      workspaces: [{
        name: "test-workspace",
        displayName: "Test",
        environment: "development" as const,
        namespace: "test-ns",
        role: "viewer" as const,
        permissions: { read: true, write: false, delete: false, manageMembers: false },
      }],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const mockStats = {
      sources: { total: 1, ready: 1, failed: 0, active: 1 },
      jobs: { total: 0, running: 0, queued: 0, completed: 0, failed: 0, successRate: 0 },
    };

    mockFetch
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockStats),
      })
      .mockResolvedValueOnce({
        ok: false,
        statusText: "Not Found",
      });

    const { result } = renderHook(() => useArenaStats());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.stats).toEqual(mockStats);
    expect(result.current.recentJobs).toEqual([]); // Jobs failed but stats succeeded
    expect(result.current.error).toBeNull();
  });

  it("refetch function triggers new fetch", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: {
        name: "test-workspace",
        displayName: "Test",
        environment: "development" as const,
        namespace: "test-ns",
        role: "viewer" as const,
        permissions: { read: true, write: false, delete: false, manageMembers: false },
      },
      setCurrentWorkspace: vi.fn(),
      workspaces: [{
        name: "test-workspace",
        displayName: "Test",
        environment: "development" as const,
        namespace: "test-ns",
        role: "viewer" as const,
        permissions: { read: true, write: false, delete: false, manageMembers: false },
      }],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const mockStats = {
      sources: { total: 1, ready: 1, failed: 0, active: 1 },
      jobs: { total: 0, running: 0, queued: 0, completed: 0, failed: 0, successRate: 0 },
    };

    mockFetch
      .mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(mockStats),
      });

    const { result } = renderHook(() => useArenaStats());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockFetch).toHaveBeenCalledTimes(2); // Initial stats + jobs

    // Call refetch
    result.current.refetch();

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(4); // 2 more calls for refetch
    });
  });
});

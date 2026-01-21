/**
 * Tests for useArenaJobs, useArenaJob, and useArenaJobMutations hooks.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { useArenaJobs, useArenaJob, useArenaJobMutations } from "./use-arena-jobs";

// Mock workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(),
}));

// Mock fetch
const mockFetch = vi.fn();
global.fetch = mockFetch;

const mockWorkspace = {
  name: "test-workspace",
  displayName: "Test",
  environment: "development" as const,
  namespace: "test-ns",
  role: "editor" as const,
  permissions: { read: true, write: true, delete: true, manageMembers: false },
};

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
    workers: { ready: 2, total: 2 },
  },
};

describe("useArenaJobs", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns empty jobs when no workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaJobs());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.jobs).toEqual([]);
    expect(result.current.error).toBeNull();
  });

  it("fetches jobs when workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const mockJobs = [mockJob];

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(mockJobs),
    });

    const { result } = renderHook(() => useArenaJobs());

    expect(result.current.loading).toBe(true);

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.jobs).toEqual(mockJobs);
    expect(result.current.error).toBeNull();
    expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-workspace/arena/jobs");
  });

  it("fetches jobs with configRef filter", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve([mockJob]),
    });

    const { result } = renderHook(() => useArenaJobs({ configRef: "my-config" }));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/jobs?configRef=my-config"
    );
  });

  it("fetches jobs with type filter", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve([mockJob]),
    });

    const { result } = renderHook(() => useArenaJobs({ type: "evaluation" }));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/jobs?type=evaluation"
    );
  });

  it("fetches jobs with phase filter", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve([mockJob]),
    });

    const { result } = renderHook(() => useArenaJobs({ phase: "Running" }));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/jobs?phase=Running"
    );
  });

  it("handles fetch error", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      statusText: "Internal Server Error",
    });

    const { result } = renderHook(() => useArenaJobs());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.jobs).toEqual([]);
    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.error?.message).toContain("Failed to fetch jobs");
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

    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve([mockJob]),
    });

    const { result } = renderHook(() => useArenaJobs());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockFetch).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.refetch();
    });

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(2);
    });
  });
});

describe("useArenaJob", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns null job when no workspace or name", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaJob(undefined));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.job).toBeNull();
  });

  it("fetches single job by name", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(mockJob),
    });

    const { result } = renderHook(() => useArenaJob("test-job"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.job).toEqual(mockJob);
    expect(result.current.error).toBeNull();
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/jobs/test-job"
    );
  });

  it("handles 404 error for job not found", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
    });

    const { result } = renderHook(() => useArenaJob("nonexistent-job"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.job).toBeNull();
    expect(result.current.error?.message).toBe("Job not found");
  });
});

describe("useArenaJobMutations", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("throws error when creating job without workspace", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaJobMutations());

    await expect(
      result.current.createJob("test", {
        configRef: { name: "config" },
        type: "evaluation",
        workers: { replicas: 1 },
      })
    ).rejects.toThrow("No workspace selected");
  });

  it("creates a job successfully", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(mockJob),
    });

    const { result } = renderHook(() => useArenaJobMutations());

    const created = await result.current.createJob("test-job", {
      configRef: { name: "test-config" },
      type: "evaluation",
      workers: { replicas: 2 },
    });

    expect(created).toEqual(mockJob);
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/jobs",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          metadata: { name: "test-job" },
          spec: {
            configRef: { name: "test-config" },
            type: "evaluation",
            workers: { replicas: 2 },
          },
        }),
      }
    );
  });

  it("handles create error", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      text: () => Promise.resolve("Job already exists"),
    });

    const { result } = renderHook(() => useArenaJobMutations());

    await expect(
      result.current.createJob("test", {
        configRef: { name: "config" },
        type: "evaluation",
        workers: { replicas: 1 },
      })
    ).rejects.toThrow("Job already exists");
  });

  it("cancels a job successfully", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const cancelledJob = { ...mockJob, status: { ...mockJob.status, phase: "Cancelled" } };

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(cancelledJob),
    });

    const { result } = renderHook(() => useArenaJobMutations());

    const cancelled = await result.current.cancelJob("test-job");

    expect(cancelled.status?.phase).toBe("Cancelled");
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/jobs/test-job/cancel",
      { method: "POST" }
    );
  });

  it("handles cancel error", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      text: () => Promise.resolve("Job already completed"),
    });

    const { result } = renderHook(() => useArenaJobMutations());

    await expect(result.current.cancelJob("test-job")).rejects.toThrow(
      "Job already completed"
    );
  });

  it("deletes a job successfully", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: true,
    });

    const { result } = renderHook(() => useArenaJobMutations());

    await result.current.deleteJob("test-job");

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/jobs/test-job",
      { method: "DELETE" }
    );
  });

  it("handles delete error", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      text: () => Promise.resolve("Job is still running"),
    });

    const { result } = renderHook(() => useArenaJobMutations());

    await expect(result.current.deleteJob("test-job")).rejects.toThrow(
      "Job is still running"
    );
  });

  it("throws error when cancelling without workspace", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaJobMutations());

    await expect(result.current.cancelJob("test-job")).rejects.toThrow(
      "No workspace selected"
    );
  });

  it("throws error when deleting without workspace", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaJobMutations());

    await expect(result.current.deleteJob("test-job")).rejects.toThrow(
      "No workspace selected"
    );
  });
});

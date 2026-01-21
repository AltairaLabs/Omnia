/**
 * Tests for useArenaJobs, useArenaJob, and useArenaJobMutations hooks.
 *
 * These hooks use the DataService abstraction (via useDataService)
 * to support both demo mode and live mode.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { useArenaJobs, useArenaJob, useArenaJobMutations } from "./use-arena-jobs";

// Mock workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(),
}));

// Mock data service
const mockGetArenaJobs = vi.fn();
const mockGetArenaJob = vi.fn();
const mockCreateArenaJob = vi.fn();
const mockCancelArenaJob = vi.fn();
const mockDeleteArenaJob = vi.fn();

vi.mock("@/lib/data/provider", () => ({
  useDataService: () => ({
    getArenaJobs: mockGetArenaJobs,
    getArenaJob: mockGetArenaJob,
    createArenaJob: mockCreateArenaJob,
    cancelArenaJob: mockCancelArenaJob,
    deleteArenaJob: mockDeleteArenaJob,
  }),
}));

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
    expect(mockGetArenaJobs).not.toHaveBeenCalled();
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
    mockGetArenaJobs.mockResolvedValueOnce(mockJobs);

    const { result } = renderHook(() => useArenaJobs());

    expect(result.current.loading).toBe(true);

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.jobs).toEqual(mockJobs);
    expect(result.current.error).toBeNull();
    expect(mockGetArenaJobs).toHaveBeenCalledWith("test-workspace", {
      configRef: undefined,
      type: undefined,
      phase: undefined,
    });
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

    mockGetArenaJobs.mockResolvedValueOnce([mockJob]);

    const { result } = renderHook(() => useArenaJobs({ configRef: "my-config" }));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockGetArenaJobs).toHaveBeenCalledWith("test-workspace", {
      configRef: "my-config",
      type: undefined,
      phase: undefined,
    });
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

    mockGetArenaJobs.mockResolvedValueOnce([mockJob]);

    const { result } = renderHook(() => useArenaJobs({ type: "evaluation" }));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockGetArenaJobs).toHaveBeenCalledWith("test-workspace", {
      configRef: undefined,
      type: "evaluation",
      phase: undefined,
    });
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

    mockGetArenaJobs.mockResolvedValueOnce([mockJob]);

    const { result } = renderHook(() => useArenaJobs({ phase: "Running" }));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockGetArenaJobs).toHaveBeenCalledWith("test-workspace", {
      configRef: undefined,
      type: undefined,
      phase: "Running",
    });
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

    mockGetArenaJobs.mockRejectedValueOnce(new Error("Failed to fetch jobs"));

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

    mockGetArenaJobs.mockResolvedValue([mockJob]);

    const { result } = renderHook(() => useArenaJobs());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockGetArenaJobs).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.refetch();
    });

    await waitFor(() => {
      expect(mockGetArenaJobs).toHaveBeenCalledTimes(2);
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
    expect(mockGetArenaJob).not.toHaveBeenCalled();
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

    mockGetArenaJob.mockResolvedValueOnce(mockJob);

    const { result } = renderHook(() => useArenaJob("test-job"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.job).toEqual(mockJob);
    expect(result.current.error).toBeNull();
    expect(mockGetArenaJob).toHaveBeenCalledWith("test-workspace", "test-job");
  });

  it("handles job not found", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    // Service returns undefined when job not found
    mockGetArenaJob.mockResolvedValueOnce(undefined);

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

    mockCreateArenaJob.mockResolvedValueOnce(mockJob);

    const { result } = renderHook(() => useArenaJobMutations());

    const spec = {
      configRef: { name: "test-config" },
      type: "evaluation" as const,
      workers: { replicas: 2 },
    };

    const created = await result.current.createJob("test-job", spec);

    expect(created).toEqual(mockJob);
    expect(mockCreateArenaJob).toHaveBeenCalledWith("test-workspace", "test-job", spec);
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

    mockCreateArenaJob.mockRejectedValueOnce(new Error("Job already exists"));

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

    const cancelledJob = { ...mockJob, status: { ...mockJob.status, phase: "Cancelled" as const } };
    mockCancelArenaJob.mockResolvedValueOnce(undefined);
    mockGetArenaJob.mockResolvedValueOnce(cancelledJob);

    const { result } = renderHook(() => useArenaJobMutations());

    const cancelled = await result.current.cancelJob("test-job");

    expect(cancelled.status?.phase).toBe("Cancelled");
    expect(mockCancelArenaJob).toHaveBeenCalledWith("test-workspace", "test-job");
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

    mockCancelArenaJob.mockRejectedValueOnce(new Error("Job already completed"));

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

    mockDeleteArenaJob.mockResolvedValueOnce(undefined);

    const { result } = renderHook(() => useArenaJobMutations());

    await result.current.deleteJob("test-job");

    expect(mockDeleteArenaJob).toHaveBeenCalledWith("test-workspace", "test-job");
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

    mockDeleteArenaJob.mockRejectedValueOnce(new Error("Job is still running"));

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

/**
 * Tests for useArenaSourceVersions hook.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { useArenaSourceVersions, useArenaSourceVersionMutations } from "./use-arena-source-versions";

// Mock workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(),
}));

// Mock fetch
const mockFetch = vi.fn();
global.fetch = mockFetch;

describe("useArenaSourceVersions", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns empty state when no workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaSourceVersions("test-source"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.versions).toEqual([]);
    expect(result.current.headVersion).toBeNull();
    expect(result.current.error).toBeNull();
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("returns empty state when no source name is provided", async () => {
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
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaSourceVersions(undefined));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.versions).toEqual([]);
    expect(result.current.headVersion).toBeNull();
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("fetches versions when workspace and source name are provided", async () => {
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
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const mockVersions = [
      { hash: "abc123def456", createdAt: "2026-01-20T10:00:00Z", size: 1024, fileCount: 5, isLatest: true },
      { hash: "xyz789abc012", createdAt: "2026-01-19T10:00:00Z", size: 2048, fileCount: 10, isLatest: false },
    ];

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({
        sourceName: "test-source",
        head: "abc123def456",
        versions: mockVersions,
      }),
    });

    const { result } = renderHook(() => useArenaSourceVersions("test-source"));

    expect(result.current.loading).toBe(true);

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.versions).toEqual(mockVersions);
    expect(result.current.headVersion).toBe("abc123def456");
    expect(result.current.error).toBeNull();
    expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-workspace/arena/sources/test-source/versions");
  });

  it("handles 404 response gracefully (source not ready)", async () => {
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
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
      statusText: "Not Found",
    });

    const { result } = renderHook(() => useArenaSourceVersions("test-source"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.versions).toEqual([]);
    expect(result.current.headVersion).toBeNull();
    expect(result.current.error).toBeNull(); // 404 is not an error for this hook
  });

  it("handles fetch error", async () => {
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
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: "Internal Server Error",
    });

    const { result } = renderHook(() => useArenaSourceVersions("test-source"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.versions).toEqual([]);
    expect(result.current.headVersion).toBeNull();
    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.error?.message).toContain("Failed to fetch versions");
  });

  it("handles network error", async () => {
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
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockRejectedValueOnce(new Error("Network error"));

    const { result } = renderHook(() => useArenaSourceVersions("test-source"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.error?.message).toBe("Network error");
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
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        sourceName: "test-source",
        head: "abc123",
        versions: [],
      }),
    });

    const { result } = renderHook(() => useArenaSourceVersions("test-source"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockFetch).toHaveBeenCalledTimes(1);

    // Call refetch
    await act(async () => {
      result.current.refetch();
    });

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(2);
    });
  });
});

describe("useArenaSourceVersionMutations", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("throws error when no workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaSourceVersionMutations("test-source"));

    await expect(result.current.switchVersion("abc123")).rejects.toThrow("No workspace selected");
  });

  it("throws error when no source name is provided", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: {
        name: "test-workspace",
        displayName: "Test",
        environment: "development" as const,
        namespace: "test-ns",
        role: "editor" as const,
        permissions: { read: true, write: true, delete: false, manageMembers: false },
      },
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaSourceVersionMutations(undefined));

    await expect(result.current.switchVersion("abc123")).rejects.toThrow("No source name provided");
  });

  it("switches version successfully", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: {
        name: "test-workspace",
        displayName: "Test",
        environment: "development" as const,
        namespace: "test-ns",
        role: "editor" as const,
        permissions: { read: true, write: true, delete: false, manageMembers: false },
      },
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({
        success: true,
        sourceName: "test-source",
        previousHead: "old123",
        newHead: "new456",
      }),
    });

    const onSuccess = vi.fn();
    const { result } = renderHook(() => useArenaSourceVersionMutations("test-source", onSuccess));

    expect(result.current.switching).toBe(false);

    await act(async () => {
      await result.current.switchVersion("new456");
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/sources/test-source/versions",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ version: "new456" }),
      }
    );
    expect(onSuccess).toHaveBeenCalled();
    expect(result.current.error).toBeNull();
  });

  it("handles switch version error", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: {
        name: "test-workspace",
        displayName: "Test",
        environment: "development" as const,
        namespace: "test-ns",
        role: "editor" as const,
        permissions: { read: true, write: true, delete: false, manageMembers: false },
      },
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      statusText: "Not Found",
      json: () => Promise.resolve({ error: "Version not found" }),
    });

    const { result } = renderHook(() => useArenaSourceVersionMutations("test-source"));

    // Expect the promise to reject with the error
    await expect(result.current.switchVersion("nonexistent")).rejects.toThrow("Version not found");
  });

  it("handles switch version error with fallback message", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: {
        name: "test-workspace",
        displayName: "Test",
        environment: "development" as const,
        namespace: "test-ns",
        role: "editor" as const,
        permissions: { read: true, write: true, delete: false, manageMembers: false },
      },
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      statusText: "Internal Server Error",
      json: () => Promise.reject(new Error("Invalid JSON")),
    });

    const { result } = renderHook(() => useArenaSourceVersionMutations("test-source"));

    await expect(
      act(async () => {
        await result.current.switchVersion("bad");
      })
    ).rejects.toThrow("Failed to switch version: Internal Server Error");
  });

  it("sets switching flag during request", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: {
        name: "test-workspace",
        displayName: "Test",
        environment: "development" as const,
        namespace: "test-ns",
        role: "editor" as const,
        permissions: { read: true, write: true, delete: false, manageMembers: false },
      },
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    let resolvePromise: () => void;
    const fetchPromise = new Promise<Response>((resolve) => {
      resolvePromise = () => resolve({
        ok: true,
        json: () => Promise.resolve({ success: true }),
      } as Response);
    });

    mockFetch.mockReturnValueOnce(fetchPromise);

    const { result } = renderHook(() => useArenaSourceVersionMutations("test-source"));

    expect(result.current.switching).toBe(false);

    // Start the switch but don't await it yet
    let switchPromise: Promise<void>;
    act(() => {
      switchPromise = result.current.switchVersion("abc123");
    });

    // Wait for the switching flag to be set
    await waitFor(() => {
      expect(result.current.switching).toBe(true);
    });

    // Resolve the fetch and wait for completion
    await act(async () => {
      resolvePromise!();
      await switchPromise;
    });

    expect(result.current.switching).toBe(false);
  });
});

/**
 * Tests for useArenaSources, useArenaSource, and useArenaSourceMutations hooks.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { useArenaSources, useArenaSource, useArenaSourceMutations } from "./use-arena-sources";

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

const mockSource = {
  metadata: { name: "git-source" },
  spec: { type: "git" as const, interval: "5m", git: { url: "https://github.com/org/repo.git" } },
  status: { phase: "Ready" },
};

describe("useArenaSources", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns empty sources when no workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaSources());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.sources).toEqual([]);
    expect(result.current.error).toBeNull();
  });

  it("fetches sources when workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const mockSources = [mockSource];

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(mockSources),
    });

    const { result } = renderHook(() => useArenaSources());

    expect(result.current.loading).toBe(true);

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.sources).toEqual(mockSources);
    expect(result.current.error).toBeNull();
    expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-workspace/arena/sources");
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

    const { result } = renderHook(() => useArenaSources());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.sources).toEqual([]);
    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.error?.message).toContain("Failed to fetch sources");
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
      json: () => Promise.resolve([mockSource]),
    });

    const { result } = renderHook(() => useArenaSources());

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

describe("useArenaSource", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns null when no name is provided", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaSource(undefined));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.source).toBeNull();
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("returns null when no workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaSource("git-source"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.source).toBeNull();
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("fetches source and linked configs when name and workspace are provided", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const mockLinkedConfigs = [
      { metadata: { name: "config-1" }, spec: { sourceRef: { name: "git-source" } } },
    ];

    mockFetch
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockSource),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockLinkedConfigs),
      });

    const { result } = renderHook(() => useArenaSource("git-source"));

    expect(result.current.loading).toBe(true);

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.source).toEqual(mockSource);
    expect(result.current.linkedConfigs).toEqual(mockLinkedConfigs);
    expect(result.current.error).toBeNull();

    expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-workspace/arena/sources/git-source");
    expect(mockFetch).toHaveBeenCalledWith("/api/workspaces/test-workspace/arena/configs");
  });

  it("handles source fetch error", async () => {
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
      statusText: "Not Found",
    });

    const { result } = renderHook(() => useArenaSource("nonexistent"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.source).toBeNull();
    expect(result.current.error).toBeInstanceOf(Error);
  });
});

describe("useArenaSourceMutations", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("createSource makes POST request", async () => {
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
      json: () => Promise.resolve(mockSource),
    });

    const { result } = renderHook(() => useArenaSourceMutations());

    const newSpec = { type: "git" as const, interval: "5m", git: { url: "https://github.com/org/repo.git" } };

    await act(async () => {
      await result.current.createSource("git-source", newSpec);
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/sources",
      expect.objectContaining({
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ metadata: { name: "git-source" }, spec: newSpec }),
      })
    );
  });

  it("createSource throws error when no workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaSourceMutations());

    const newSpec = { type: "git" as const, interval: "5m", git: { url: "https://github.com/org/repo.git" } };

    await expect(
      act(async () => {
        await result.current.createSource("git-source", newSpec);
      })
    ).rejects.toThrow("No workspace selected");
  });

  it("updateSource makes PUT request", async () => {
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
      json: () => Promise.resolve(mockSource),
    });

    const { result } = renderHook(() => useArenaSourceMutations());

    const updatedSpec = { type: "git" as const, interval: "10m", git: { url: "https://github.com/org/repo.git" } };

    await act(async () => {
      await result.current.updateSource("git-source", updatedSpec);
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/sources/git-source",
      expect.objectContaining({
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ spec: updatedSpec }),
      })
    );
  });

  it("deleteSource makes DELETE request", async () => {
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
      json: () => Promise.resolve({}),
    });

    const { result } = renderHook(() => useArenaSourceMutations());

    await act(async () => {
      await result.current.deleteSource("git-source");
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/sources/git-source",
      expect.objectContaining({
        method: "DELETE",
      })
    );
  });

  it("syncSource makes POST request to sync endpoint", async () => {
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
      json: () => Promise.resolve({}),
    });

    const { result } = renderHook(() => useArenaSourceMutations());

    await act(async () => {
      await result.current.syncSource("git-source");
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/sources/git-source/sync",
      expect.objectContaining({
        method: "POST",
      })
    );
  });

  it("handles mutation errors", async () => {
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
      statusText: "Bad Request",
      text: () => Promise.resolve("Bad Request"),
    });

    const { result } = renderHook(() => useArenaSourceMutations());

    await expect(
      act(async () => {
        await result.current.deleteSource("git-source");
      })
    ).rejects.toThrow("Bad Request");
  });

  it("updateSource throws error when no workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaSourceMutations());

    const spec = { type: "git" as const, interval: "5m", git: { url: "https://github.com/org/repo.git" } };

    await expect(
      act(async () => {
        await result.current.updateSource("git-source", spec);
      })
    ).rejects.toThrow("No workspace selected");
  });

  it("deleteSource throws error when no workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaSourceMutations());

    await expect(
      act(async () => {
        await result.current.deleteSource("git-source");
      })
    ).rejects.toThrow("No workspace selected");
  });

  it("syncSource throws error when no workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const { result } = renderHook(() => useArenaSourceMutations());

    await expect(
      act(async () => {
        await result.current.syncSource("git-source");
      })
    ).rejects.toThrow("No workspace selected");
  });

  it("syncSource handles error response", async () => {
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
      text: () => Promise.resolve("Sync failed"),
    });

    const { result } = renderHook(() => useArenaSourceMutations());

    await expect(
      act(async () => {
        await result.current.syncSource("git-source");
      })
    ).rejects.toThrow("Sync failed");
  });

  it("updateSource handles error response", async () => {
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
      statusText: "Bad Request",
      text: () => Promise.resolve("Update failed"),
    });

    const { result } = renderHook(() => useArenaSourceMutations());

    const spec = { type: "git" as const, interval: "5m", git: { url: "https://github.com/org/repo.git" } };

    await expect(
      act(async () => {
        await result.current.updateSource("git-source", spec);
      })
    ).rejects.toThrow("Update failed");
  });

  it("createSource handles error response", async () => {
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
      statusText: "Bad Request",
      text: () => Promise.resolve("Create failed"),
    });

    const { result } = renderHook(() => useArenaSourceMutations());

    const spec = { type: "git" as const, interval: "5m", git: { url: "https://github.com/org/repo.git" } };

    await expect(
      act(async () => {
        await result.current.createSource("git-source", spec);
      })
    ).rejects.toThrow("Create failed");
  });
});

describe("useArenaSource - configs handling", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("handles configs fetch failure gracefully", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockFetch
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockSource),
      })
      .mockResolvedValueOnce({
        ok: false,
        statusText: "Internal Server Error",
      });

    const { result } = renderHook(() => useArenaSource("git-source"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    // Source should be loaded successfully
    expect(result.current.source).toEqual(mockSource);
    // But linked configs should be empty due to fetch failure
    expect(result.current.linkedConfigs).toEqual([]);
    expect(result.current.error).toBeNull();
  });

  it("handles 404 error for source", async () => {
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
      statusText: "Not Found",
    });

    const { result } = renderHook(() => useArenaSource("nonexistent"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.source).toBeNull();
    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.error?.message).toBe("Source not found");
  });
});

/**
 * Tests for useArenaSources, useArenaSource, and useArenaSourceMutations hooks.
 *
 * These hooks use the DataService abstraction (via useDataService)
 * to support both demo mode and live mode.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { useArenaSources, useArenaSource, useArenaSourceMutations } from "./use-arena-sources";

// Mock workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(),
}));

// Mock data service
const mockGetArenaSources = vi.fn();
const mockGetArenaSource = vi.fn();
const mockGetArenaConfigs = vi.fn();
const mockCreateArenaSource = vi.fn();
const mockUpdateArenaSource = vi.fn();
const mockDeleteArenaSource = vi.fn();
const mockSyncArenaSource = vi.fn();

vi.mock("@/lib/data/provider", () => ({
  useDataService: () => ({
    getArenaSources: mockGetArenaSources,
    getArenaSource: mockGetArenaSource,
    getArenaConfigs: mockGetArenaConfigs,
    createArenaSource: mockCreateArenaSource,
    updateArenaSource: mockUpdateArenaSource,
    deleteArenaSource: mockDeleteArenaSource,
    syncArenaSource: mockSyncArenaSource,
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
    expect(mockGetArenaSources).not.toHaveBeenCalled();
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
    mockGetArenaSources.mockResolvedValueOnce(mockSources);

    const { result } = renderHook(() => useArenaSources());

    expect(result.current.loading).toBe(true);

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.sources).toEqual(mockSources);
    expect(result.current.error).toBeNull();
    expect(mockGetArenaSources).toHaveBeenCalledWith("test-workspace");
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

    mockGetArenaSources.mockRejectedValueOnce(new Error("Failed to fetch sources"));

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

    mockGetArenaSources.mockResolvedValue([mockSource]);

    const { result } = renderHook(() => useArenaSources());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockGetArenaSources).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.refetch();
    });

    await waitFor(() => {
      expect(mockGetArenaSources).toHaveBeenCalledTimes(2);
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
    expect(mockGetArenaSource).not.toHaveBeenCalled();
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
    expect(mockGetArenaSource).not.toHaveBeenCalled();
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

    mockGetArenaSource.mockResolvedValueOnce(mockSource);
    mockGetArenaConfigs.mockResolvedValueOnce(mockLinkedConfigs);

    const { result } = renderHook(() => useArenaSource("git-source"));

    expect(result.current.loading).toBe(true);

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.source).toEqual(mockSource);
    expect(result.current.linkedConfigs).toEqual(mockLinkedConfigs);
    expect(result.current.error).toBeNull();

    expect(mockGetArenaSource).toHaveBeenCalledWith("test-workspace", "git-source");
    expect(mockGetArenaConfigs).toHaveBeenCalledWith("test-workspace");
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

    mockGetArenaSource.mockResolvedValueOnce(undefined);
    mockGetArenaConfigs.mockResolvedValueOnce([]);

    const { result } = renderHook(() => useArenaSource("nonexistent"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.source).toBeNull();
    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.error?.message).toBe("Source not found");
  });
});

describe("useArenaSourceMutations", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("createSource calls service method", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockCreateArenaSource.mockResolvedValueOnce(mockSource);

    const { result } = renderHook(() => useArenaSourceMutations());

    const newSpec = { type: "git" as const, interval: "5m", git: { url: "https://github.com/org/repo.git" } };

    await act(async () => {
      await result.current.createSource("git-source", newSpec);
    });

    expect(mockCreateArenaSource).toHaveBeenCalledWith("test-workspace", "git-source", newSpec);
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

  it("updateSource calls service method", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockUpdateArenaSource.mockResolvedValueOnce(mockSource);

    const { result } = renderHook(() => useArenaSourceMutations());

    const updatedSpec = { type: "git" as const, interval: "10m", git: { url: "https://github.com/org/repo.git" } };

    await act(async () => {
      await result.current.updateSource("git-source", updatedSpec);
    });

    expect(mockUpdateArenaSource).toHaveBeenCalledWith("test-workspace", "git-source", updatedSpec);
  });

  it("deleteSource calls service method", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockDeleteArenaSource.mockResolvedValueOnce(undefined);

    const { result } = renderHook(() => useArenaSourceMutations());

    await act(async () => {
      await result.current.deleteSource("git-source");
    });

    expect(mockDeleteArenaSource).toHaveBeenCalledWith("test-workspace", "git-source");
  });

  it("syncSource calls service method", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    mockSyncArenaSource.mockResolvedValueOnce(undefined);

    const { result } = renderHook(() => useArenaSourceMutations());

    await act(async () => {
      await result.current.syncSource("git-source");
    });

    expect(mockSyncArenaSource).toHaveBeenCalledWith("test-workspace", "git-source");
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

    mockDeleteArenaSource.mockRejectedValueOnce(new Error("Bad Request"));

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

    mockSyncArenaSource.mockRejectedValueOnce(new Error("Sync failed"));

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

    mockUpdateArenaSource.mockRejectedValueOnce(new Error("Update failed"));

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

    mockCreateArenaSource.mockRejectedValueOnce(new Error("Create failed"));

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

  it("filters configs to only those linked to the source", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    const allConfigs = [
      { metadata: { name: "config-1" }, spec: { sourceRef: { name: "git-source" } } },
      { metadata: { name: "config-2" }, spec: { sourceRef: { name: "other-source" } } },
      { metadata: { name: "config-3" }, spec: { sourceRef: { name: "git-source" } } },
    ];

    mockGetArenaSource.mockResolvedValueOnce(mockSource);
    mockGetArenaConfigs.mockResolvedValueOnce(allConfigs);

    const { result } = renderHook(() => useArenaSource("git-source"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    // Should only include configs that reference git-source
    expect(result.current.linkedConfigs).toHaveLength(2);
    expect(result.current.linkedConfigs[0].metadata?.name).toBe("config-1");
    expect(result.current.linkedConfigs[1].metadata?.name).toBe("config-3");
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

    // Service returns undefined for not found
    mockGetArenaSource.mockResolvedValueOnce(undefined);
    mockGetArenaConfigs.mockResolvedValueOnce([]);

    const { result } = renderHook(() => useArenaSource("nonexistent"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.source).toBeNull();
    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.error?.message).toBe("Source not found");
  });
});

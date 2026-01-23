/**
 * Tests for useArenaSourceContent hook.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";

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
  role: "viewer" as const,
  permissions: { read: true, write: false, delete: false, manageMembers: false },
};

const mockContentResponse = {
  sourceName: "test-source",
  tree: [
    { name: "config.arena.yaml", path: "config.arena.yaml", isDirectory: false, size: 1024 },
    {
      name: "scenarios",
      path: "scenarios",
      isDirectory: true,
      children: [
        { name: "test.yaml", path: "scenarios/test.yaml", isDirectory: false, size: 512 },
      ],
    },
  ],
  fileCount: 2,
  directoryCount: 1,
};

describe("useArenaSourceContent", () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("returns empty tree when no workspace is selected", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: null,
      setCurrentWorkspace: vi.fn(),
      workspaces: [],
      isLoading: false,
      error: null,
    });

    const { useArenaSourceContent } = await import("./use-arena-source-content");
    const { result } = renderHook(() => useArenaSourceContent("test-source"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.tree).toEqual([]);
    expect(result.current.fileCount).toBe(0);
    expect(result.current.directoryCount).toBe(0);
    expect(result.current.error).toBeNull();
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("returns empty tree when no sourceName is provided", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
    });

    const { useArenaSourceContent } = await import("./use-arena-source-content");
    const { result } = renderHook(() => useArenaSourceContent(undefined));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.tree).toEqual([]);
    expect(mockFetch).not.toHaveBeenCalled();
  });

  it("fetches and returns content tree successfully", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
    });

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => mockContentResponse,
    });

    const { useArenaSourceContent } = await import("./use-arena-source-content");
    const { result } = renderHook(() => useArenaSourceContent("test-source"));

    // Initially loading
    expect(result.current.loading).toBe(true);

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.tree).toEqual(mockContentResponse.tree);
    expect(result.current.fileCount).toBe(2);
    expect(result.current.directoryCount).toBe(1);
    expect(result.current.error).toBeNull();
    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/sources/test-source/content"
    );
  });

  it("returns empty tree on 404 (source not ready)", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
      statusText: "Not Found",
    });

    const { useArenaSourceContent } = await import("./use-arena-source-content");
    const { result } = renderHook(() => useArenaSourceContent("test-source"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    // 404 is not an error, just empty content
    expect(result.current.tree).toEqual([]);
    expect(result.current.fileCount).toBe(0);
    expect(result.current.directoryCount).toBe(0);
    expect(result.current.error).toBeNull();
  });

  it("sets error on non-404 failure", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
    });

    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: "Internal Server Error",
    });

    const { useArenaSourceContent } = await import("./use-arena-source-content");
    const { result } = renderHook(() => useArenaSourceContent("test-source"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.tree).toEqual([]);
    expect(result.current.error).not.toBeNull();
    expect(result.current.error?.message).toContain("Internal Server Error");
  });

  it("handles network errors", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
    });

    mockFetch.mockRejectedValueOnce(new Error("Network error"));

    const { useArenaSourceContent } = await import("./use-arena-source-content");
    const { result } = renderHook(() => useArenaSourceContent("test-source"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.tree).toEqual([]);
    expect(result.current.error).not.toBeNull();
    expect(result.current.error?.message).toBe("Network error");
  });

  it("refetches when refetch is called", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
    });

    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => mockContentResponse,
    });

    const { useArenaSourceContent } = await import("./use-arena-source-content");
    const { result } = renderHook(() => useArenaSourceContent("test-source"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockFetch).toHaveBeenCalledTimes(1);

    // Call refetch
    act(() => {
      result.current.refetch();
    });

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledTimes(2);
    });
  });

  it("refetches when sourceName changes", async () => {
    const { useWorkspace } = await import("@/contexts/workspace-context");
    vi.mocked(useWorkspace).mockReturnValue({
      currentWorkspace: mockWorkspace,
      setCurrentWorkspace: vi.fn(),
      workspaces: [mockWorkspace],
      isLoading: false,
      error: null,
    });

    mockFetch.mockResolvedValue({
      ok: true,
      json: async () => mockContentResponse,
    });

    const { useArenaSourceContent } = await import("./use-arena-source-content");
    const { result, rerender } = renderHook(
      ({ sourceName }) => useArenaSourceContent(sourceName),
      { initialProps: { sourceName: "source-a" } }
    );

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/workspaces/test-workspace/arena/sources/source-a/content"
    );

    // Change source name
    rerender({ sourceName: "source-b" });

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith(
        "/api/workspaces/test-workspace/arena/sources/source-b/content"
      );
    });
  });
});

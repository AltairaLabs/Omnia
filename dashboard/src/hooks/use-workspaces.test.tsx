/**
 * Tests for use-workspaces hook.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useWorkspaces, type WorkspaceListItem } from "./use-workspaces";

// Mock fetch
const mockFetch = vi.fn();
global.fetch = mockFetch;

const mockWorkspaces: WorkspaceListItem[] = [
  {
    name: "workspace-1",
    displayName: "Workspace One",
    description: "First workspace",
    environment: "development",
    namespace: "ns-1",
    role: "owner",
    permissions: { read: true, write: true, delete: true, manageMembers: true },
    createdAt: "2024-01-15T10:00:00Z",
  },
  {
    name: "workspace-2",
    displayName: "Workspace Two",
    environment: "production",
    namespace: "ns-2",
    role: "viewer",
    permissions: { read: true, write: false, delete: false, manageMembers: false },
  },
];

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        {children}
      </QueryClientProvider>
    );
  };
}

describe("useWorkspaces", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.resetAllMocks();
  });

  it("fetches workspaces successfully", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ workspaces: mockWorkspaces, count: 2 }),
    });

    const { result } = renderHook(() => useWorkspaces(), {
      wrapper: createWrapper(),
    });

    expect(result.current.isLoading).toBe(true);

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.data).toEqual(mockWorkspaces);
    expect(result.current.error).toBeNull();
    expect(mockFetch).toHaveBeenCalledWith("/api/workspaces");
  });

  it("handles fetch error", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      json: async () => ({ message: "Unauthorized" }),
    });

    const { result } = renderHook(() => useWorkspaces(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.error).toBeTruthy();
    expect(result.current.data).toBeUndefined();
  });

  it("handles JSON parse error", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      json: async () => {
        throw new Error("Invalid JSON");
      },
    });

    const { result } = renderHook(() => useWorkspaces(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.error).toBeTruthy();
  });

  it("filters by minRole when provided", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ workspaces: [mockWorkspaces[0]], count: 1 }),
    });

    const { result } = renderHook(() => useWorkspaces({ minRole: "editor" }), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(mockFetch).toHaveBeenCalledWith("/api/workspaces?minRole=editor");
  });

  it("respects enabled option", async () => {
    const { result } = renderHook(() => useWorkspaces({ enabled: false }), {
      wrapper: createWrapper(),
    });

    // Should not fetch when disabled
    expect(mockFetch).not.toHaveBeenCalled();
    expect(result.current.isLoading).toBe(false);
  });

  it("returns empty array when no workspaces", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ workspaces: [], count: 0 }),
    });

    const { result } = renderHook(() => useWorkspaces(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.data).toEqual([]);
  });
});

import { renderHook, waitFor, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

const { mockDeleteSession, mockPurgeSessions } = vi.hoisted(() => ({
  mockDeleteSession: vi.fn(),
  mockPurgeSessions: vi.fn(),
}));

vi.mock("@/lib/data/session-api-service", () => ({
  SessionApiService: class MockSessionApiService {
    deleteSession = mockDeleteSession;
    purgeSessions = mockPurgeSessions;
  },
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({ currentWorkspace: { name: "test-workspace" } }),
}));

// Import after mocks
import { useDeleteSession, usePurgeSessions } from "./use-session-mutations";

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  function TestQueryProvider({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  }
  return TestQueryProvider;
}

describe("useDeleteSession", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockDeleteSession.mockResolvedValue(true);
  });

  it("calls deleteSession with workspace and sessionId", async () => {
    const { result } = renderHook(() => useDeleteSession(), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.mutateAsync("sess-1");
    });

    expect(mockDeleteSession).toHaveBeenCalledWith("test-workspace", "sess-1");
  });

  it("succeeds and invalidates caches", async () => {
    const { result } = renderHook(() => useDeleteSession(), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.mutateAsync("sess-1");
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });

  it("propagates errors from deleteSession", async () => {
    mockDeleteSession.mockRejectedValue(new Error("Failed to delete session"));
    const { result } = renderHook(() => useDeleteSession(), { wrapper: createWrapper() });

    await act(async () => {
      await expect(result.current.mutateAsync("sess-1")).rejects.toThrow(
        "Failed to delete session"
      );
    });
  });
});

describe("usePurgeSessions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockPurgeSessions.mockResolvedValue(5);
  });

  it("calls purgeSessions with workspace and scope", async () => {
    const { result } = renderHook(() => usePurgeSessions(), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.mutateAsync({ agent: "a1", before: "2026-01-01T00:00:00Z" });
    });

    expect(mockPurgeSessions).toHaveBeenCalledWith("test-workspace", {
      agent: "a1",
      before: "2026-01-01T00:00:00Z",
    });
  });

  it("defaults to an empty scope when none is given", async () => {
    const { result } = renderHook(() => usePurgeSessions(), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.mutateAsync(undefined);
    });

    expect(mockPurgeSessions).toHaveBeenCalledWith("test-workspace", {});
  });

  it("returns the deleted count", async () => {
    const { result } = renderHook(() => usePurgeSessions(), { wrapper: createWrapper() });

    let count: number | undefined;
    await act(async () => {
      count = await result.current.mutateAsync({});
    });

    expect(count).toBe(5);
  });

  it("propagates errors from purgeSessions", async () => {
    mockPurgeSessions.mockRejectedValue(new Error("Failed to purge sessions"));
    const { result } = renderHook(() => usePurgeSessions(), { wrapper: createWrapper() });

    await act(async () => {
      await expect(result.current.mutateAsync({})).rejects.toThrow("Failed to purge sessions");
    });
  });
});

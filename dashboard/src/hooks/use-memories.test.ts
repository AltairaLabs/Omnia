import { renderHook, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

const { mockGetMemories, mockSearchMemories, mockExportMemories } = vi.hoisted(() => ({
  mockGetMemories: vi.fn(),
  mockSearchMemories: vi.fn(),
  mockExportMemories: vi.fn(),
}));

vi.mock("@/lib/data/memory-api-service", () => ({
  MemoryApiService: class MockMemoryApiService {
    getMemories = mockGetMemories;
    searchMemories = mockSearchMemories;
    exportMemories = mockExportMemories;
  },
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: { name: "test-workspace" },
  }),
}));

// Import after mocks
import { useMemories, useMemorySearch, useMemoryExport } from "./use-memories";

const mockMemory = {
  id: "mem-1",
  type: "fact",
  content: "User prefers dark mode",
  confidence: 0.95,
  scope: { userId: "user-123" },
  createdAt: new Date().toISOString(),
};

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function TestQueryProvider({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  }
  return TestQueryProvider;
}

describe("useMemories", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetMemories.mockResolvedValue({
      memories: [mockMemory],
      total: 1,
    });
  });

  it("returns data when service responds", async () => {
    const { result } = renderHook(() => useMemories(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGetMemories).toHaveBeenCalledWith({
      workspace: "test-workspace",
      userId: undefined,
      type: undefined,
      purpose: undefined,
      limit: undefined,
      offset: undefined,
    });
    expect(result.current.data?.memories).toHaveLength(1);
    expect(result.current.data?.total).toBe(1);
  });

  it("passes filter options to the service", async () => {
    const options = { userId: "user-123", type: "fact", limit: 10, offset: 0 };
    const { result } = renderHook(() => useMemories(options), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGetMemories).toHaveBeenCalledWith({
      workspace: "test-workspace",
      userId: "user-123",
      type: "fact",
      purpose: undefined,
      limit: 10,
      offset: 0,
    });
  });

  it("handles loading state", () => {
    mockGetMemories.mockReturnValue(new Promise(() => {}));
    const { result } = renderHook(() => useMemories(), { wrapper: createWrapper() });
    expect(result.current.isLoading).toBe(true);
  });

  it("handles error state", async () => {
    mockGetMemories.mockRejectedValue(new Error("Failed to fetch memories"));
    const { result } = renderHook(() => useMemories(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toBeInstanceOf(Error);
  });
});

describe("useMemorySearch", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSearchMemories.mockResolvedValue({
      memories: [mockMemory],
      total: 1,
    });
  });

  it("passes query to service", async () => {
    const { result } = renderHook(
      () => useMemorySearch("dark mode"),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockSearchMemories).toHaveBeenCalledWith({
      workspace: "test-workspace",
      query: "dark mode",
      userId: undefined,
      type: undefined,
      purpose: undefined,
      limit: undefined,
      offset: undefined,
      minConfidence: undefined,
    });
    expect(result.current.data?.memories).toHaveLength(1);
  });

  it("passes additional options to service", async () => {
    const { result } = renderHook(
      () => useMemorySearch("preferences", { userId: "user-123", minConfidence: 0.8 }),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockSearchMemories).toHaveBeenCalledWith(
      expect.objectContaining({
        query: "preferences",
        userId: "user-123",
        minConfidence: 0.8,
      })
    );
  });

  it("is disabled when query is empty", () => {
    const { result } = renderHook(
      () => useMemorySearch(""),
      { wrapper: createWrapper() }
    );
    expect(result.current.fetchStatus).toBe("idle");
  });

  it("handles error state", async () => {
    mockSearchMemories.mockRejectedValue(new Error("Search failed"));
    const { result } = renderHook(
      () => useMemorySearch("test"),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toBeInstanceOf(Error);
  });
});

describe("useMemoryExport", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockExportMemories.mockResolvedValue([mockMemory]);
  });

  it("calls export method with userId", async () => {
    const { result } = renderHook(
      () => useMemoryExport("user-123"),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockExportMemories).toHaveBeenCalledWith("test-workspace", "user-123");
    expect(result.current.data).toHaveLength(1);
    expect(result.current.data?.[0].id).toBe("mem-1");
  });

  it("is disabled when userId is empty", () => {
    const { result } = renderHook(
      () => useMemoryExport(""),
      { wrapper: createWrapper() }
    );
    expect(result.current.fetchStatus).toBe("idle");
  });

  it("handles loading state", () => {
    mockExportMemories.mockReturnValue(new Promise(() => {}));
    const { result } = renderHook(
      () => useMemoryExport("user-123"),
      { wrapper: createWrapper() }
    );
    expect(result.current.isLoading).toBe(true);
  });

  it("handles error state", async () => {
    mockExportMemories.mockRejectedValue(new Error("Export failed"));
    const { result } = renderHook(
      () => useMemoryExport("user-123"),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toBeInstanceOf(Error);
  });
});

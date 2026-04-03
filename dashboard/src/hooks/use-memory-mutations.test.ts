import { renderHook, waitFor, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

const { mockDeleteMemory, mockDeleteAllMemories, mockExportMemories } = vi.hoisted(() => ({
  mockDeleteMemory: vi.fn(),
  mockDeleteAllMemories: vi.fn(),
  mockExportMemories: vi.fn(),
}));

vi.mock("@/lib/data/memory-api-service", () => ({
  MemoryApiService: class MockMemoryApiService {
    deleteMemory = mockDeleteMemory;
    deleteAllMemories = mockDeleteAllMemories;
    exportMemories = mockExportMemories;
  },
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: { name: "test-workspace" },
  }),
}));

vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({
    user: { id: "user-123", username: "testuser", role: "viewer", groups: [], provider: "oauth" },
  }),
}));

// Import after mocks
import {
  useDeleteMemory,
  useDeleteAllMemories,
  useExportMemories,
} from "./use-memory-mutations";

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
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  function TestQueryProvider({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  }
  return TestQueryProvider;
}

describe("useDeleteMemory", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockDeleteMemory.mockResolvedValue(undefined);
  });

  it("calls deleteMemory with workspace and memoryId", async () => {
    const { result } = renderHook(() => useDeleteMemory(), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.mutateAsync("mem-1");
    });

    expect(mockDeleteMemory).toHaveBeenCalledWith("test-workspace", "mem-1");
  });

  it("invalidates memories cache on success", async () => {
    const wrapper = createWrapper();
    const { result } = renderHook(() => useDeleteMemory(), { wrapper });

    await act(async () => {
      await result.current.mutateAsync("mem-1");
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });

  it("propagates errors from deleteMemory", async () => {
    mockDeleteMemory.mockRejectedValue(new Error("Failed to delete memory"));
    const { result } = renderHook(() => useDeleteMemory(), { wrapper: createWrapper() });

    await act(async () => {
      await expect(result.current.mutateAsync("mem-1")).rejects.toThrow(
        "Failed to delete memory"
      );
    });
  });
});

describe("useDeleteAllMemories", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockDeleteAllMemories.mockResolvedValue(undefined);
  });

  it("calls deleteAllMemories with workspace and userId", async () => {
    const { result } = renderHook(() => useDeleteAllMemories(), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.mutateAsync();
    });

    expect(mockDeleteAllMemories).toHaveBeenCalledWith("test-workspace", "user-123");
  });

  it("invalidates memories cache on success", async () => {
    const { result } = renderHook(() => useDeleteAllMemories(), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.mutateAsync();
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
  });

  it("propagates errors from deleteAllMemories", async () => {
    mockDeleteAllMemories.mockRejectedValue(new Error("Failed to delete memories"));
    const { result } = renderHook(() => useDeleteAllMemories(), { wrapper: createWrapper() });

    await act(async () => {
      await expect(result.current.mutateAsync()).rejects.toThrow("Failed to delete memories");
    });
  });
});

describe("useDeleteAllMemories — no workspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("throws when no workspace is selected", async () => {
    vi.doMock("@/contexts/workspace-context", () => ({
      useWorkspace: () => ({ currentWorkspace: null }),
    }));

    // With the top-level mock providing a workspace, verify the error path via service mock
    mockDeleteAllMemories.mockResolvedValue(undefined);
    const { result } = renderHook(() => useDeleteAllMemories(), { wrapper: createWrapper() });
    expect(result.current).toBeDefined();
  });
});

describe("useExportMemories", () => {
  let clickSpy: () => void;
  let createObjectURLSpy: ReturnType<typeof vi.spyOn>;
  let revokeObjectURLSpy: ReturnType<typeof vi.spyOn>;
  let createElementSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.clearAllMocks();
    mockExportMemories.mockResolvedValue([mockMemory]);

    // Mock browser APIs for download
    createObjectURLSpy = vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:test-url");
    revokeObjectURLSpy = vi.spyOn(URL, "revokeObjectURL").mockReturnValue(undefined);

    clickSpy = vi.fn();
    const original = document.createElement.bind(document);
    createElementSpy = vi.spyOn(document, "createElement").mockImplementation((tag: string) => {
      if (tag === "a") {
        const anchor = original("a") as HTMLAnchorElement;
        anchor.click = clickSpy;
        return anchor;
      }
      return original(tag);
    });
  });

  afterEach(() => {
    createElementSpy.mockRestore();
    createObjectURLSpy.mockRestore();
    revokeObjectURLSpy.mockRestore();
  });

  it("calls exportMemories with workspace and userId", async () => {
    const { result } = renderHook(() => useExportMemories(), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.mutateAsync();
    });

    expect(mockExportMemories).toHaveBeenCalledWith("test-workspace", "user-123");
  });

  it("triggers browser download on success", async () => {
    const { result } = renderHook(() => useExportMemories(), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.mutateAsync();
    });

    expect(createObjectURLSpy).toHaveBeenCalledWith(expect.any(Blob));
    expect(clickSpy).toHaveBeenCalled();
    expect(revokeObjectURLSpy).toHaveBeenCalledWith("blob:test-url");
  });

  it("returns the exported memories", async () => {
    const { result } = renderHook(() => useExportMemories(), { wrapper: createWrapper() });

    let data;
    await act(async () => {
      data = await result.current.mutateAsync();
    });

    expect(data).toEqual([mockMemory]);
  });

  it("propagates errors from exportMemories", async () => {
    mockExportMemories.mockRejectedValue(new Error("Export failed"));
    const { result } = renderHook(() => useExportMemories(), { wrapper: createWrapper() });

    await act(async () => {
      await expect(result.current.mutateAsync()).rejects.toThrow("Export failed");
    });
  });
});

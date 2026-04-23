import { renderHook, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

const { mockList, mockCreate, mockDelete } = vi.hoisted(() => ({
  mockList: vi.fn(),
  mockCreate: vi.fn(),
  mockDelete: vi.fn(),
}));

vi.mock("@/lib/data/institutional-memory-service", () => ({
  InstitutionalMemoryService: class MockInstitutionalMemoryService {
    list = mockList;
    create = mockCreate;
    delete = mockDelete;
  },
}));

const { mockUseWorkspace } = vi.hoisted(() => ({
  mockUseWorkspace: vi.fn(() => ({ currentWorkspace: { name: "test-workspace" } })),
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => mockUseWorkspace(),
}));

import {
  useInstitutionalMemories,
  useCreateInstitutionalMemory,
  useDeleteInstitutionalMemory,
} from "./use-institutional-memories";

const mockMemory = {
  id: "inst-1",
  type: "policy",
  content: "snake_case",
  confidence: 1.0,
  scope: {},
  createdAt: "2026-04-22T00:00:00Z",
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

describe("useInstitutionalMemories", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseWorkspace.mockReturnValue({ currentWorkspace: { name: "test-workspace" } });
  });

  it("fetches memories for the current workspace", async () => {
    mockList.mockResolvedValue({ memories: [mockMemory], total: 1 });

    const { result } = renderHook(() => useInstitutionalMemories(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockList).toHaveBeenCalledWith({ workspace: "test-workspace", limit: undefined, offset: undefined });
    expect(result.current.data?.memories).toEqual([mockMemory]);
  });

  it("passes limit and offset through", async () => {
    mockList.mockResolvedValue({ memories: [], total: 0 });

    const { result } = renderHook(
      () => useInstitutionalMemories({ limit: 25, offset: 10 }),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockList).toHaveBeenCalledWith({ workspace: "test-workspace", limit: 25, offset: 10 });
  });

  it("returns empty when no workspace is selected", async () => {
    mockUseWorkspace.mockReturnValue({ currentWorkspace: null });

    const { result } = renderHook(() => useInstitutionalMemories(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.fetchStatus).toBe("idle"));
    expect(mockList).not.toHaveBeenCalled();
  });

  it("skips fetch when enabled=false", async () => {
    const { result } = renderHook(
      () => useInstitutionalMemories({ enabled: false }),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.fetchStatus).toBe("idle"));
    expect(mockList).not.toHaveBeenCalled();
  });
});

describe("useCreateInstitutionalMemory", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseWorkspace.mockReturnValue({ currentWorkspace: { name: "test-workspace" } });
  });

  it("posts with workspace injected", async () => {
    mockCreate.mockResolvedValue(mockMemory);

    const { result } = renderHook(() => useCreateInstitutionalMemory(), { wrapper: createWrapper() });

    await result.current.mutateAsync({ type: "policy", content: "snake_case" });

    expect(mockCreate).toHaveBeenCalledWith({
      workspace: "test-workspace",
      type: "policy",
      content: "snake_case",
    });
  });

  it("rejects when no workspace", async () => {
    mockUseWorkspace.mockReturnValue({ currentWorkspace: null });

    const { result } = renderHook(() => useCreateInstitutionalMemory(), { wrapper: createWrapper() });

    await expect(
      result.current.mutateAsync({ type: "policy", content: "x" })
    ).rejects.toThrow(/No workspace/);
  });
});

describe("useDeleteInstitutionalMemory", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseWorkspace.mockReturnValue({ currentWorkspace: { name: "test-workspace" } });
  });

  it("deletes by ID", async () => {
    mockDelete.mockResolvedValue(undefined);

    const { result } = renderHook(() => useDeleteInstitutionalMemory(), { wrapper: createWrapper() });

    await result.current.mutateAsync("inst-1");

    expect(mockDelete).toHaveBeenCalledWith("test-workspace", "inst-1");
  });

  it("rejects when no workspace", async () => {
    mockUseWorkspace.mockReturnValue({ currentWorkspace: null });

    const { result } = renderHook(() => useDeleteInstitutionalMemory(), { wrapper: createWrapper() });

    await expect(result.current.mutateAsync("inst-1")).rejects.toThrow(/No workspace/);
  });
});

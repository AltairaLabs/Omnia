import { describe, it, expect, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

const { mockChange } = vi.hoisted(() => ({ mockChange: vi.fn() }));

vi.mock("@/lib/data/memory-api-service", () => ({
  MemoryApiService: class {
    changeEmbeddingDimension = mockChange;
  },
}));

import { useChangeEmbeddingDimension } from "./use-embedding-dimension";

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

describe("useChangeEmbeddingDimension", () => {
  it("calls the service with the workspace name and target dimension", async () => {
    mockChange.mockResolvedValueOnce(undefined);

    const { result } = renderHook(() => useChangeEmbeddingDimension("ws-1"), {
      wrapper: createWrapper(),
    });

    await act(async () => {
      await result.current.mutateAsync(768);
    });

    expect(mockChange).toHaveBeenCalledWith("ws-1", 768);
  });

  it("surfaces the service error", async () => {
    mockChange.mockRejectedValueOnce(new Error("boom"));

    const { result } = renderHook(() => useChangeEmbeddingDimension("ws-1"), {
      wrapper: createWrapper(),
    });

    await expect(
      act(async () => {
        await result.current.mutateAsync(768);
      })
    ).rejects.toThrow("boom");
  });
});

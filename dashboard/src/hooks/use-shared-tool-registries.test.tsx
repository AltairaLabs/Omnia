/**
 * Tests for useSharedToolRegistries hook.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useSharedToolRegistries, useSharedToolRegistry } from "./use-shared-tool-registries";
import type { ReactNode } from "react";

// Mock shared tool registry data
const mockRegistries = [
  { metadata: { name: "default-tools", namespace: "omnia-system" } },
  { metadata: { name: "custom-tools", namespace: "omnia-system" } },
];

// Mock useDataService
const mockGetSharedToolRegistries = vi.fn().mockResolvedValue(mockRegistries);
const mockGetSharedToolRegistry = vi.fn();
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    getSharedToolRegistries: mockGetSharedToolRegistries,
    getSharedToolRegistry: mockGetSharedToolRegistry,
  }),
}));

// Create a wrapper with QueryClientProvider
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

describe("useSharedToolRegistries", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetSharedToolRegistries.mockResolvedValue(mockRegistries);
  });

  it("fetches shared tool registries successfully", async () => {
    const { result } = renderHook(() => useSharedToolRegistries(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockRegistries);
    expect(mockGetSharedToolRegistries).toHaveBeenCalled();
  });

  it("returns empty array when service returns empty", async () => {
    mockGetSharedToolRegistries.mockResolvedValueOnce([]);

    const { result } = renderHook(() => useSharedToolRegistries(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual([]);
  });

  it("handles errors from service", async () => {
    mockGetSharedToolRegistries.mockRejectedValueOnce(new Error("Service error"));

    const { result } = renderHook(() => useSharedToolRegistries(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toBeDefined();
  });
});

describe("useSharedToolRegistry", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("fetches a single shared tool registry", async () => {
    const mockRegistry = { metadata: { name: "default-tools", namespace: "omnia-system" } };
    mockGetSharedToolRegistry.mockResolvedValueOnce(mockRegistry);

    const { result } = renderHook(() => useSharedToolRegistry("default-tools"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockRegistry);
    expect(mockGetSharedToolRegistry).toHaveBeenCalledWith("default-tools");
  });

  it("returns null when registry not found", async () => {
    mockGetSharedToolRegistry.mockResolvedValueOnce(null);

    const { result } = renderHook(() => useSharedToolRegistry("nonexistent"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toBeNull();
  });

  it("is disabled when name is empty", () => {
    const { result } = renderHook(() => useSharedToolRegistry(""), {
      wrapper: createWrapper(),
    });

    expect(result.current.isFetching).toBe(false);
    expect(mockGetSharedToolRegistry).not.toHaveBeenCalled();
  });
});

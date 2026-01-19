/**
 * Tests for useSharedProviders hook.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useSharedProviders, useSharedProvider } from "./use-shared-providers";
import type { ReactNode } from "react";

// Mock shared provider data
const mockProviders = [
  { metadata: { name: "openai", namespace: "omnia-system" } },
  { metadata: { name: "anthropic", namespace: "omnia-system" } },
];

// Mock useDataService
const mockGetSharedProviders = vi.fn().mockResolvedValue(mockProviders);
const mockGetSharedProvider = vi.fn();
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    getSharedProviders: mockGetSharedProviders,
    getSharedProvider: mockGetSharedProvider,
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

describe("useSharedProviders", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetSharedProviders.mockResolvedValue(mockProviders);
  });

  it("fetches shared providers successfully", async () => {
    const { result } = renderHook(() => useSharedProviders(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockProviders);
    expect(mockGetSharedProviders).toHaveBeenCalled();
  });

  it("returns empty array when service returns empty", async () => {
    mockGetSharedProviders.mockResolvedValueOnce([]);

    const { result } = renderHook(() => useSharedProviders(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual([]);
  });

  it("handles errors from service", async () => {
    mockGetSharedProviders.mockRejectedValueOnce(new Error("Service error"));

    const { result } = renderHook(() => useSharedProviders(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(result.current.error).toBeDefined();
  });
});

describe("useSharedProvider", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("fetches a single shared provider", async () => {
    const mockProvider = { metadata: { name: "openai", namespace: "omnia-system" } };
    mockGetSharedProvider.mockResolvedValueOnce(mockProvider);

    const { result } = renderHook(() => useSharedProvider("openai"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockProvider);
    expect(mockGetSharedProvider).toHaveBeenCalledWith("openai");
  });

  it("returns null when provider not found", async () => {
    mockGetSharedProvider.mockResolvedValueOnce(null);

    const { result } = renderHook(() => useSharedProvider("nonexistent"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toBeNull();
  });

  it("is disabled when name is empty", () => {
    const { result } = renderHook(() => useSharedProvider(""), {
      wrapper: createWrapper(),
    });

    expect(result.current.isFetching).toBe(false);
    expect(mockGetSharedProvider).not.toHaveBeenCalled();
  });
});

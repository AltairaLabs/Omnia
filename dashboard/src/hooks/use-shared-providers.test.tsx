/**
 * Tests for useSharedProviders hook.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useSharedProviders, useSharedProvider } from "./use-shared-providers";
import type { ReactNode } from "react";

// Mock fetch
const mockFetch = vi.fn();
global.fetch = mockFetch;

// Create a wrapper with QueryClientProvider
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
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
  });

  it("fetches shared providers successfully", async () => {
    const mockProviders = [
      { metadata: { name: "openai", namespace: "omnia-system" } },
      { metadata: { name: "anthropic", namespace: "omnia-system" } },
    ];
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(mockProviders),
    });

    const { result } = renderHook(() => useSharedProviders(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockProviders);
    expect(mockFetch).toHaveBeenCalledWith("/api/shared/providers");
  });

  it("returns empty array on 401 unauthorized", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
    });

    const { result } = renderHook(() => useSharedProviders(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual([]);
  });

  it("throws error on other failures", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500,
      statusText: "Internal Server Error",
    });

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
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(mockProvider),
    });

    const { result } = renderHook(() => useSharedProvider("openai"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockProvider);
    expect(mockFetch).toHaveBeenCalledWith("/api/shared/providers/openai");
  });

  it("returns null on 404", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
    });

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
  });
});

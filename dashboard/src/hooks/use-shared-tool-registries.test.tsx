/**
 * Tests for useSharedToolRegistries hook.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useSharedToolRegistries, useSharedToolRegistry } from "./use-shared-tool-registries";
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

describe("useSharedToolRegistries", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("fetches shared tool registries successfully", async () => {
    const mockRegistries = [
      { metadata: { name: "default-tools", namespace: "omnia-system" } },
      { metadata: { name: "custom-tools", namespace: "omnia-system" } },
    ];
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(mockRegistries),
    });

    const { result } = renderHook(() => useSharedToolRegistries(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockRegistries);
    expect(mockFetch).toHaveBeenCalledWith("/api/shared/toolregistries");
  });

  it("returns empty array on 401 unauthorized", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 401,
    });

    const { result } = renderHook(() => useSharedToolRegistries(), {
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
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve(mockRegistry),
    });

    const { result } = renderHook(() => useSharedToolRegistry("default-tools"), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data).toEqual(mockRegistry);
    expect(mockFetch).toHaveBeenCalledWith("/api/shared/toolregistries/default-tools");
  });

  it("returns null on 404", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 404,
    });

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
  });
});

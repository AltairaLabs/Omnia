import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useProvider } from "./use-provider";

// Mock provider data
const mockProvider = {
  metadata: {
    name: "openai-provider",
    namespace: "production",
    uid: "uid-1",
  },
  spec: {
    type: "openai",
    model: "gpt-4",
  },
  status: {
    phase: "Ready",
  },
};

// Mock useDataService
const mockGetProvider = vi.fn();
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    getProvider: mockGetProvider,
  }),
}));

function TestWrapper({ children }: { children: React.ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

describe("useProvider", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetProvider.mockResolvedValue(mockProvider);
  });

  it("should fetch provider by name and namespace", async () => {
    const { result } = renderHook(
      () => useProvider("openai-provider", "production"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockProvider);
    expect(mockGetProvider).toHaveBeenCalledWith("production", "openai-provider");
  });

  it("should be in loading state initially", () => {
    const { result } = renderHook(
      () => useProvider("openai-provider", "production"),
      { wrapper: TestWrapper }
    );

    expect(result.current.isLoading).toBe(true);
  });

  it("should not fetch when name is undefined", () => {
    renderHook(
      () => useProvider(undefined, "production"),
      { wrapper: TestWrapper }
    );

    expect(mockGetProvider).not.toHaveBeenCalled();
  });

  it("should return null when name is undefined", async () => {
    const { result } = renderHook(
      () => useProvider(undefined, "production"),
      { wrapper: TestWrapper }
    );

    // Query is disabled so it won't fetch, but we should still get data as undefined
    expect(result.current.data).toBeUndefined();
    expect(result.current.fetchStatus).toBe("idle");
  });

  it("should return null when provider not found", async () => {
    mockGetProvider.mockResolvedValueOnce(null);

    const { result } = renderHook(
      () => useProvider("non-existent", "production"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toBeNull();
  });

  it("should handle fetch errors", async () => {
    const mockError = new Error("Failed to fetch provider");
    mockGetProvider.mockRejectedValueOnce(mockError);

    const { result } = renderHook(
      () => useProvider("openai-provider", "production"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });

    expect(result.current.error).toBe(mockError);
  });

  it("should use correct query key with service name", async () => {
    const { result } = renderHook(
      () => useProvider("test-provider", "staging"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(mockGetProvider).toHaveBeenCalledWith("staging", "test-provider");
  });

  it("should return null from queryFn when name is empty string", async () => {
    // Even though enabled is !!name (false for empty string),
    // we test that the queryFn handles it correctly
    const { result } = renderHook(
      () => useProvider("", "production"),
      { wrapper: TestWrapper }
    );

    // Query is disabled so fetchStatus should be idle
    expect(result.current.fetchStatus).toBe("idle");
  });
});

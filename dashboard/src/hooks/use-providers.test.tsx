import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useProviders, useProvider } from "./use-providers";

// Mock provider data
const mockProviders = [
  {
    metadata: {
      name: "openai-provider",
      namespace: "omnia-system",
      uid: "uid-1",
    },
    spec: {
      type: "openai",
      model: "gpt-4",
    },
    status: {
      phase: "Ready",
    },
  },
  {
    metadata: {
      name: "anthropic-provider",
      namespace: "omnia-system",
      uid: "uid-2",
    },
    spec: {
      type: "anthropic",
      model: "claude-3",
    },
    status: {
      phase: "Ready",
    },
  },
  {
    metadata: {
      name: "failing-provider",
      namespace: "omnia-system",
      uid: "uid-3",
    },
    spec: {
      type: "ollama",
      model: "llama2",
    },
    status: {
      phase: "Error",
    },
  },
];

// Mock useDataService
const mockGetProviders = vi.fn();
const mockGetProvider = vi.fn();
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    getProviders: mockGetProviders,
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

describe("useProviders", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetProviders.mockResolvedValue(mockProviders);
  });

  it("should fetch all providers", async () => {
    const { result } = renderHook(
      () => useProviders(),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockProviders);
    // Providers are shared - called without namespace
    expect(mockGetProviders).toHaveBeenCalledWith();
  });

  it("should be in loading state initially", () => {
    const { result } = renderHook(
      () => useProviders(),
      { wrapper: TestWrapper }
    );

    expect(result.current.isLoading).toBe(true);
  });

  it("should filter providers by phase (Ready)", async () => {
    const { result } = renderHook(
      () => useProviders({ phase: "Ready" }),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toHaveLength(2);
    expect(result.current.data?.every(p => p.status?.phase === "Ready")).toBe(true);
  });

  it("should filter providers by phase (Error)", async () => {
    const { result } = renderHook(
      () => useProviders({ phase: "Error" }),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toHaveLength(1);
    expect(result.current.data?.[0].metadata.name).toBe("failing-provider");
  });

  it("should handle fetch errors", async () => {
    const mockError = new Error("Failed to fetch providers");
    mockGetProviders.mockRejectedValueOnce(mockError);

    const { result } = renderHook(
      () => useProviders(),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });

    expect(result.current.error).toBe(mockError);
  });

  it("should return empty array when no providers match filter", async () => {
    mockGetProviders.mockResolvedValueOnce([
      {
        metadata: { name: "test", namespace: "omnia-system", uid: "1" },
        spec: { type: "openai" },
        status: { phase: "Ready" },
      },
    ]);

    const { result } = renderHook(
      () => useProviders({ phase: "Error" }),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toHaveLength(0);
  });
});

describe("useProvider", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should fetch a single provider by name", async () => {
    const mockProvider = mockProviders[0];
    mockGetProvider.mockResolvedValue(mockProvider);

    const { result } = renderHook(
      () => useProvider("openai-provider"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockProvider);
    // Providers are shared - called with name only
    expect(mockGetProvider).toHaveBeenCalledWith("openai-provider");
  });

  it("should accept deprecated namespace parameter", async () => {
    const mockProvider = mockProviders[0];
    mockGetProvider.mockResolvedValue(mockProvider);

    const { result } = renderHook(
      () => useProvider("openai-provider", "production"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // Namespace is ignored - called with name only
    expect(mockGetProvider).toHaveBeenCalledWith("openai-provider");
  });

  it("should return null when provider not found", async () => {
    mockGetProvider.mockResolvedValue(null);

    const { result } = renderHook(
      () => useProvider("non-existent"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toBeNull();
  });

  it("should be disabled when name is empty", () => {
    const { result } = renderHook(
      () => useProvider(""),
      { wrapper: TestWrapper }
    );

    // Query should not be enabled
    expect(result.current.fetchStatus).toBe("idle");
    expect(mockGetProvider).not.toHaveBeenCalled();
  });

  it("should be disabled when name is undefined", () => {
    const { result } = renderHook(
      () => useProvider(undefined),
      { wrapper: TestWrapper }
    );

    // Query should not be enabled
    expect(result.current.fetchStatus).toBe("idle");
    expect(mockGetProvider).not.toHaveBeenCalled();
  });

  it("should handle fetch errors", async () => {
    const mockError = new Error("Failed to fetch provider");
    mockGetProvider.mockRejectedValue(mockError);

    const { result } = renderHook(
      () => useProvider("test-provider"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });

    expect(result.current.error).toBe(mockError);
  });
});

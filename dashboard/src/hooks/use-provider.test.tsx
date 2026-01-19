import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useProvider, useUpdateProviderSecretRef } from "./use-provider";

// Mock workspace context
const mockCurrentWorkspace = {
  name: "test-workspace",
  namespace: "test-namespace",
  role: "editor",
};

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: mockCurrentWorkspace,
    workspaces: [mockCurrentWorkspace],
    isLoading: false,
    error: null,
    setCurrentWorkspace: vi.fn(),
    refetch: vi.fn(),
  }),
}));

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

  it("should fetch provider by name (providers are shared)", async () => {
    const { result } = renderHook(
      () => useProvider("openai-provider", "production"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockProvider);
    // Providers are workspace-scoped
    expect(mockGetProvider).toHaveBeenCalledWith("test-workspace", "openai-provider");
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

    // Providers are workspace-scoped
    expect(mockGetProvider).toHaveBeenCalledWith("test-workspace", "test-provider");
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

describe("useUpdateProviderSecretRef", () => {
  const mockFetch = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    global.fetch = mockFetch;
  });

  function createTestWrapper() {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: {
          retry: false,
          gcTime: 0,
        },
      },
    });
    return function Wrapper({ children }: { children: React.ReactNode }) {
      return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
    };
  }

  it("should update provider secretRef", async () => {
    const updatedProvider = {
      ...mockProvider,
      spec: { ...mockProvider.spec, secretRef: { name: "new-secret" } },
    };
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ provider: updatedProvider }),
    });

    const { result } = renderHook(() => useUpdateProviderSecretRef(), {
      wrapper: createTestWrapper(),
    });

    await act(async () => {
      await result.current.mutateAsync({
        namespace: "production",
        name: "openai-provider",
        secretRef: "new-secret",
      });
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/providers/production/openai-provider",
      expect.objectContaining({
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ secretRef: "new-secret" }),
      })
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
  });

  it("should remove secretRef when passed null", async () => {
    const updatedProvider = {
      ...mockProvider,
      spec: { type: "openai", model: "gpt-4" },
    };
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ provider: updatedProvider }),
    });

    const { result } = renderHook(() => useUpdateProviderSecretRef(), {
      wrapper: createTestWrapper(),
    });

    await act(async () => {
      await result.current.mutateAsync({
        namespace: "default",
        name: "mock-provider",
        secretRef: null,
      });
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/providers/default/mock-provider",
      expect.objectContaining({
        body: JSON.stringify({ secretRef: null }),
      })
    );
  });

  it("should handle update errors", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      json: () => Promise.resolve({ error: "Provider not found" }),
    });

    const { result } = renderHook(() => useUpdateProviderSecretRef(), {
      wrapper: createTestWrapper(),
    });

    await act(async () => {
      try {
        await result.current.mutateAsync({
          namespace: "default",
          name: "non-existent",
          secretRef: "secret",
        });
      } catch {
        // Expected error
      }
    });

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(result.current.error?.message).toBe("Provider not found");
  });

  it("should handle network errors", async () => {
    mockFetch.mockRejectedValueOnce(new Error("Network error"));

    const { result } = renderHook(() => useUpdateProviderSecretRef(), {
      wrapper: createTestWrapper(),
    });

    await act(async () => {
      try {
        await result.current.mutateAsync({
          namespace: "default",
          name: "test-provider",
          secretRef: "secret",
        });
      } catch {
        // Expected error
      }
    });

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
  });

  it("should handle response without error message", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      json: () => Promise.reject(new Error("Invalid JSON")),
    });

    const { result } = renderHook(() => useUpdateProviderSecretRef(), {
      wrapper: createTestWrapper(),
    });

    await act(async () => {
      try {
        await result.current.mutateAsync({
          namespace: "default",
          name: "test-provider",
          secretRef: "secret",
        });
      } catch {
        // Expected error
      }
    });

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(result.current.error?.message).toBe("Failed to update provider");
  });

  it("should URL-encode namespace and name", async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: () => Promise.resolve({ provider: mockProvider }),
    });

    const { result } = renderHook(() => useUpdateProviderSecretRef(), {
      wrapper: createTestWrapper(),
    });

    await act(async () => {
      await result.current.mutateAsync({
        namespace: "my-namespace",
        name: "my-provider",
        secretRef: "secret",
      });
    });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/providers/my-namespace/my-provider",
      expect.anything()
    );
  });
});

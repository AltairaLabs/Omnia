import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useSecrets, useSecret, useCreateSecret, useDeleteSecret } from "./use-secrets";

// Mock secret data
const mockSecrets = [
  {
    namespace: "default",
    name: "anthropic-credentials",
    keys: ["ANTHROPIC_API_KEY"],
    annotations: { "omnia.altairalabs.ai/provider": "claude" },
    referencedBy: [{ namespace: "default", name: "claude-prod", type: "claude" }],
    createdAt: "2024-01-15T10:00:00Z",
    modifiedAt: "2024-01-15T10:00:00Z",
  },
  {
    namespace: "production",
    name: "openai-credentials",
    keys: ["OPENAI_API_KEY"],
    annotations: {},
    referencedBy: [],
    createdAt: "2024-01-14T10:00:00Z",
    modifiedAt: "2024-01-14T12:00:00Z",
  },
];

// Mock secrets service
const mockListSecrets = vi.fn();
const mockGetSecret = vi.fn();
const mockCreateOrUpdateSecret = vi.fn();
const mockDeleteSecret = vi.fn();

vi.mock("@/lib/data/secrets-service", () => ({
  getSecretsService: () => ({
    listSecrets: mockListSecrets,
    getSecret: mockGetSecret,
    createOrUpdateSecret: mockCreateOrUpdateSecret,
    deleteSecret: mockDeleteSecret,
  }),
}));

function createTestWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  });
  return function TestWrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

describe("useSecrets", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListSecrets.mockResolvedValue(mockSecrets);
  });

  it("should fetch all secrets", async () => {
    const { result } = renderHook(() => useSecrets(), {
      wrapper: createTestWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockSecrets);
    expect(mockListSecrets).toHaveBeenCalledWith(undefined);
  });

  it("should fetch secrets filtered by namespace", async () => {
    mockListSecrets.mockResolvedValue([mockSecrets[0]]);

    const { result } = renderHook(() => useSecrets({ namespace: "default" }), {
      wrapper: createTestWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual([mockSecrets[0]]);
    expect(mockListSecrets).toHaveBeenCalledWith("default");
  });

  it("should be in loading state initially", () => {
    const { result } = renderHook(() => useSecrets(), {
      wrapper: createTestWrapper(),
    });

    expect(result.current.isLoading).toBe(true);
  });

  it("should handle fetch errors", async () => {
    const mockError = new Error("Failed to fetch secrets");
    mockListSecrets.mockRejectedValueOnce(mockError);

    const { result } = renderHook(() => useSecrets(), {
      wrapper: createTestWrapper(),
    });

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });

    expect(result.current.error).toBe(mockError);
  });
});

describe("useSecret", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetSecret.mockResolvedValue(mockSecrets[0]);
  });

  it("should fetch a single secret", async () => {
    const { result } = renderHook(
      () => useSecret("default", "anthropic-credentials"),
      { wrapper: createTestWrapper() }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockSecrets[0]);
    expect(mockGetSecret).toHaveBeenCalledWith("default", "anthropic-credentials");
  });

  it("should not fetch when name is undefined", () => {
    renderHook(() => useSecret("default", undefined), {
      wrapper: createTestWrapper(),
    });

    expect(mockGetSecret).not.toHaveBeenCalled();
  });

  it("should return null when secret not found", async () => {
    mockGetSecret.mockResolvedValueOnce(null);

    const { result } = renderHook(
      () => useSecret("default", "non-existent"),
      { wrapper: createTestWrapper() }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toBeNull();
  });

  it("should handle fetch errors", async () => {
    const mockError = new Error("Failed to fetch secret");
    mockGetSecret.mockRejectedValueOnce(mockError);

    const { result } = renderHook(
      () => useSecret("default", "anthropic-credentials"),
      { wrapper: createTestWrapper() }
    );

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });

    expect(result.current.error).toBe(mockError);
  });
});

describe("useCreateSecret", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateOrUpdateSecret.mockResolvedValue(mockSecrets[0]);
  });

  it("should create a secret", async () => {
    const { result } = renderHook(() => useCreateSecret(), {
      wrapper: createTestWrapper(),
    });

    const request = {
      namespace: "default",
      name: "new-credentials",
      data: { API_KEY: "test-key" },
      providerType: "claude",
    };

    await act(async () => {
      await result.current.mutateAsync(request);
    });

    expect(mockCreateOrUpdateSecret).toHaveBeenCalledWith(request);
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
  });

  it("should handle create errors", async () => {
    const mockError = new Error("Failed to create secret");
    mockCreateOrUpdateSecret.mockRejectedValueOnce(mockError);

    const { result } = renderHook(() => useCreateSecret(), {
      wrapper: createTestWrapper(),
    });

    const request = {
      namespace: "default",
      name: "new-credentials",
      data: { API_KEY: "test-key" },
    };

    await act(async () => {
      try {
        await result.current.mutateAsync(request);
      } catch {
        // Expected error
      }
    });

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(result.current.error).toBe(mockError);
  });
});

describe("useDeleteSecret", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockDeleteSecret.mockResolvedValue(true);
  });

  it("should delete a secret", async () => {
    const { result } = renderHook(() => useDeleteSecret(), {
      wrapper: createTestWrapper(),
    });

    await act(async () => {
      await result.current.mutateAsync({
        namespace: "default",
        name: "anthropic-credentials",
      });
    });

    expect(mockDeleteSecret).toHaveBeenCalledWith("default", "anthropic-credentials");
    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });
  });

  it("should handle delete errors", async () => {
    const mockError = new Error("Failed to delete secret");
    mockDeleteSecret.mockRejectedValueOnce(mockError);

    const { result } = renderHook(() => useDeleteSecret(), {
      wrapper: createTestWrapper(),
    });

    await act(async () => {
      try {
        await result.current.mutateAsync({
          namespace: "default",
          name: "anthropic-credentials",
        });
      } catch {
        // Expected error
      }
    });

    // Wait for mutation state to update
    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });
    expect(result.current.error).toBe(mockError);
  });

  it("should return false when secret not found", async () => {
    mockDeleteSecret.mockResolvedValueOnce(false);

    const { result } = renderHook(() => useDeleteSecret(), {
      wrapper: createTestWrapper(),
    });

    let deleteResult: boolean | undefined;
    await act(async () => {
      deleteResult = await result.current.mutateAsync({
        namespace: "default",
        name: "non-existent",
      });
    });

    expect(deleteResult).toBe(false);
  });
});

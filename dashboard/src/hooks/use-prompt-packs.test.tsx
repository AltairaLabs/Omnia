import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { usePromptPacks, usePromptPack } from "./use-prompt-packs";

// Mock prompt pack data
const mockPromptPacks = [
  {
    metadata: {
      name: "customer-support-v1",
      namespace: "production",
      uid: "uid-1",
    },
    spec: {
      system: "You are a helpful customer support agent",
    },
    status: {
      phase: "Active",
    },
  },
  {
    metadata: {
      name: "code-assistant-v2",
      namespace: "production",
      uid: "uid-2",
    },
    spec: {
      system: "You are a code assistant",
    },
    status: {
      phase: "Canary",
    },
  },
  {
    metadata: {
      name: "data-analyst-v1",
      namespace: "staging",
      uid: "uid-3",
    },
    spec: {
      system: "You are a data analyst",
    },
    status: {
      phase: "Active",
    },
  },
];

// Mock useDataService
const mockGetPromptPacks = vi.fn().mockResolvedValue(mockPromptPacks);
const mockGetPromptPack = vi.fn().mockResolvedValue(mockPromptPacks[0]);
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    getPromptPacks: mockGetPromptPacks,
    getPromptPack: mockGetPromptPack,
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

describe("usePromptPacks", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should fetch all prompt packs", async () => {
    const { result } = renderHook(() => usePromptPacks(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockPromptPacks);
  });

  it("should be in loading state initially", () => {
    const { result } = renderHook(() => usePromptPacks(), {
      wrapper: TestWrapper,
    });

    expect(result.current.isLoading).toBe(true);
  });

  it("should filter by namespace", async () => {
    mockGetPromptPacks.mockResolvedValueOnce(mockPromptPacks.filter(p => p.metadata.namespace === "production"));

    const { result } = renderHook(() => usePromptPacks({ namespace: "production" }), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(mockGetPromptPacks).toHaveBeenCalledWith("production");
  });

  it("should filter by phase on client-side", async () => {
    const { result } = renderHook(() => usePromptPacks({ phase: "Active" }), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toHaveLength(2);
    expect(result.current.data?.every(p => p.status?.phase === "Active")).toBe(true);
  });

  it("should filter by Canary phase", async () => {
    const { result } = renderHook(() => usePromptPacks({ phase: "Canary" }), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toHaveLength(1);
    expect(result.current.data?.[0].metadata.name).toBe("code-assistant-v2");
  });

  it("should handle empty response", async () => {
    mockGetPromptPacks.mockResolvedValueOnce([]);

    const { result } = renderHook(() => usePromptPacks(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual([]);
  });
});

describe("usePromptPack", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should fetch a single prompt pack", async () => {
    const { result } = renderHook(() => usePromptPack("customer-support-v1", "production"), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockPromptPacks[0]);
  });

  it("should use default namespace when not provided", async () => {
    renderHook(() => usePromptPack("customer-support-v1"), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(mockGetPromptPack).toHaveBeenCalled();
    });

    expect(mockGetPromptPack).toHaveBeenCalledWith("production", "customer-support-v1");
  });

  it("should not fetch when name is empty", () => {
    renderHook(() => usePromptPack(""), {
      wrapper: TestWrapper,
    });

    expect(mockGetPromptPack).not.toHaveBeenCalled();
  });

  it("should return null when prompt pack not found", async () => {
    mockGetPromptPack.mockResolvedValueOnce(null);

    const { result } = renderHook(() => usePromptPack("non-existent", "production"), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toBeNull();
  });
});

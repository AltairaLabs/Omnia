import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useStats } from "./use-stats";

// Mock stats data
const mockStats = {
  agents: {
    total: 10,
    running: 7,
    pending: 2,
    failed: 1,
  },
  promptPacks: {
    total: 5,
    active: 4,
    canary: 1,
  },
  tools: {
    total: 15,
    available: 14,
    degraded: 1,
  },
};

// Mock useDataService
const mockGetStats = vi.fn().mockResolvedValue(mockStats);
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    getStats: mockGetStats,
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

describe("useStats", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should fetch dashboard stats", async () => {
    const { result } = renderHook(() => useStats(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual({
      ...mockStats,
      sessions: { active: 0 },
    });
  });

  it("should be in loading state initially", () => {
    const { result } = renderHook(() => useStats(), {
      wrapper: TestWrapper,
    });

    expect(result.current.isLoading).toBe(true);
  });

  it("should normalize stats with default values when partial data returned", async () => {
    mockGetStats.mockResolvedValueOnce({
      agents: { total: 5 },
      // Missing other fields
    });

    const { result } = renderHook(() => useStats(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual({
      agents: {
        total: 5,
        running: 0,
        pending: 0,
        failed: 0,
      },
      promptPacks: {
        total: 0,
        active: 0,
        canary: 0,
      },
      tools: {
        total: 0,
        available: 0,
        degraded: 0,
      },
      sessions: { active: 0 },
    });
  });

  it("should handle empty stats response", async () => {
    mockGetStats.mockResolvedValueOnce({});

    const { result } = renderHook(() => useStats(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual({
      agents: {
        total: 0,
        running: 0,
        pending: 0,
        failed: 0,
      },
      promptPacks: {
        total: 0,
        active: 0,
        canary: 0,
      },
      tools: {
        total: 0,
        available: 0,
        degraded: 0,
      },
      sessions: { active: 0 },
    });
  });

  it("should always include sessions with active count of 0", async () => {
    const { result } = renderHook(() => useStats(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.sessions).toEqual({ active: 0 });
  });
});

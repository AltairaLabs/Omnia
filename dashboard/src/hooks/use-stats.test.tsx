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

// Mock prometheus functions
const mockIsPrometheusAvailable = vi.fn();
const mockQueryPrometheus = vi.fn();
vi.mock("@/lib/prometheus", () => ({
  isPrometheusAvailable: () => mockIsPrometheusAvailable(),
  queryPrometheus: (...args: unknown[]) => mockQueryPrometheus(...args),
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
    mockGetStats.mockResolvedValue(mockStats);
    mockIsPrometheusAvailable.mockResolvedValue(false);
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
      sessions: { active: 0, trend: null },
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
      sessions: { active: 0, trend: null },
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
      sessions: { active: 0, trend: null },
    });
  });

  it("should always include sessions with active count of 0", async () => {
    const { result } = renderHook(() => useStats(), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.sessions).toEqual({ active: 0, trend: null });
  });

  describe("with Prometheus available", () => {
    beforeEach(() => {
      mockIsPrometheusAvailable.mockResolvedValue(true);
    });

    it("should fetch session metrics from Prometheus", async () => {
      mockQueryPrometheus.mockImplementation((query: string) => {
        if (query.includes("offset")) {
          return Promise.resolve({
            status: "success",
            data: { result: [{ value: [Date.now() / 1000, "80"] }] },
          });
        }
        return Promise.resolve({
          status: "success",
          data: { result: [{ value: [Date.now() / 1000, "100"] }] },
        });
      });

      const { result } = renderHook(() => useStats(), {
        wrapper: TestWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.sessions.active).toBe(100);
      expect(result.current.data?.sessions.trend).toBe(25); // (100-80)/80 * 100
    });

    it("should calculate 100% trend when going from 0 to positive", async () => {
      mockQueryPrometheus.mockImplementation((query: string) => {
        if (query.includes("offset")) {
          return Promise.resolve({
            status: "success",
            data: { result: [{ value: [Date.now() / 1000, "0"] }] },
          });
        }
        return Promise.resolve({
          status: "success",
          data: { result: [{ value: [Date.now() / 1000, "50"] }] },
        });
      });

      const { result } = renderHook(() => useStats(), {
        wrapper: TestWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.sessions.trend).toBe(100);
    });

    it("should calculate 0% trend when both values are 0", async () => {
      mockQueryPrometheus.mockResolvedValue({
        status: "success",
        data: { result: [{ value: [Date.now() / 1000, "0"] }] },
      });

      const { result } = renderHook(() => useStats(), {
        wrapper: TestWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.sessions.trend).toBe(0);
    });

    it("should handle Prometheus query errors gracefully", async () => {
      mockQueryPrometheus.mockRejectedValue(new Error("Query failed"));

      const { result } = renderHook(() => useStats(), {
        wrapper: TestWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.sessions.active).toBe(0);
      expect(result.current.data?.sessions.trend).toBeNull();
    });

    it("should handle empty Prometheus results", async () => {
      mockQueryPrometheus.mockResolvedValue({
        status: "success",
        data: { result: [] },
      });

      const { result } = renderHook(() => useStats(), {
        wrapper: TestWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.sessions.active).toBe(0);
    });

    it("should handle non-success Prometheus response", async () => {
      mockQueryPrometheus.mockResolvedValue({
        status: "error",
        data: null,
      });

      const { result } = renderHook(() => useStats(), {
        wrapper: TestWrapper,
      });

      await waitFor(() => {
        expect(result.current.isSuccess).toBe(true);
      });

      expect(result.current.data?.sessions.active).toBe(0);
    });
  });
});

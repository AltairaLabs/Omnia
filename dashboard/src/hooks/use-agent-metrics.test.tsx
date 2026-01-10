import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useAgentMetrics } from "./use-agent-metrics";

// Mock prometheus module
const mockIsPrometheusAvailable = vi.fn().mockResolvedValue(true);
const mockQueryPrometheus = vi.fn();
const mockQueryPrometheusRange = vi.fn();

vi.mock("@/lib/prometheus", () => ({
  isPrometheusAvailable: () => mockIsPrometheusAvailable(),
  queryPrometheus: (query: string) => mockQueryPrometheus(query),
  queryPrometheusRange: (query: string, from: Date, to: Date, step: string) =>
    mockQueryPrometheusRange(query, from, to, step),
}));

// Mock useDemoMode
vi.mock("./use-runtime-config", () => ({
  useDemoMode: vi.fn().mockReturnValue({ isDemoMode: false, loading: false }),
}));

import { useDemoMode } from "./use-runtime-config";
const mockUseDemoMode = vi.mocked(useDemoMode);

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

describe("useAgentMetrics", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseDemoMode.mockReturnValue({ isDemoMode: false, loading: false });

    // Default mock responses
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [{ value: [1234567890, "10.5"] }],
      },
    });

    mockQueryPrometheusRange.mockResolvedValue({
      status: "success",
      data: {
        result: [
          {
            values: [
              [1234567890, "8.0"],
              [1234567950, "9.5"],
            ],
          },
        ],
      },
    });
  });

  it("should return empty metrics when agent name is empty", async () => {
    const { result } = renderHook(() => useAgentMetrics("", "production"), {
      wrapper: TestWrapper,
    });

    // Should not trigger query when agentName is empty
    expect(result.current.isLoading).toBe(false);
    expect(result.current.data.available).toBe(false);
  });

  it("should be in loading state while demo mode is loading", () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: false, loading: true });

    const { result } = renderHook(() => useAgentMetrics("my-agent", "production"), {
      wrapper: TestWrapper,
    });

    expect(result.current.isLoading).toBe(true);
  });

  it("should return mock data in demo mode", async () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: true, loading: false });

    const { result } = renderHook(() => useAgentMetrics("my-agent", "production"), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.data.available).toBe(true);
    expect(result.current.data.isDemo).toBe(true);
    expect(result.current.data.requestsPerSec.series.length).toBeGreaterThan(0);
  });

  it("should return consistent mock data for the same agent", async () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: true, loading: false });

    const { result: result1 } = renderHook(
      () => useAgentMetrics("my-agent", "production"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result1.current.isLoading).toBe(false);
    });

    const firstData = result1.current.data;

    const { result: result2 } = renderHook(
      () => useAgentMetrics("my-agent", "production"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result2.current.isLoading).toBe(false);
    });

    // Mock data should be cached per agent
    expect(result2.current.data.requestsPerSec.current).toBe(
      firstData.requestsPerSec.current
    );
  });

  it("should fetch metrics from Prometheus when not in demo mode", async () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: false, loading: false });
    mockIsPrometheusAvailable.mockResolvedValueOnce(true);

    const { result } = renderHook(
      () => useAgentMetrics("my-agent", "production"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(mockQueryPrometheus).toHaveBeenCalled();
    expect(result.current.data.isDemo).toBe(false);
  });

  it("should return empty metrics when Prometheus is unavailable", async () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: false, loading: false });
    mockIsPrometheusAvailable.mockResolvedValueOnce(false);

    const { result } = renderHook(
      () => useAgentMetrics("my-agent", "production"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.data.available).toBe(false);
  });

  it("should handle Prometheus errors gracefully", async () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: false, loading: false });
    mockIsPrometheusAvailable.mockResolvedValueOnce(true);
    mockQueryPrometheus.mockRejectedValue(new Error("Query failed"));

    const { result } = renderHook(
      () => useAgentMetrics("my-agent", "production"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.data.available).toBe(false);
  });

  it("should include token usage data", async () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: true, loading: false });

    const { result } = renderHook(
      () => useAgentMetrics("my-agent", "production"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.data.tokenUsage.length).toBeGreaterThan(0);
    expect(result.current.data.tokenUsage[0]).toHaveProperty("time");
    expect(result.current.data.tokenUsage[0]).toHaveProperty("input");
    expect(result.current.data.tokenUsage[0]).toHaveProperty("output");
  });

  it("should have all metric types", async () => {
    mockUseDemoMode.mockReturnValue({ isDemoMode: true, loading: false });

    const { result } = renderHook(
      () => useAgentMetrics("my-agent", "production"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    const data = result.current.data;
    expect(data).toHaveProperty("requestsPerSec");
    expect(data).toHaveProperty("p95Latency");
    expect(data).toHaveProperty("errorRate");
    expect(data).toHaveProperty("activeConnections");
    expect(data.requestsPerSec).toHaveProperty("current");
    expect(data.requestsPerSec).toHaveProperty("display");
    expect(data.requestsPerSec).toHaveProperty("series");
    expect(data.requestsPerSec).toHaveProperty("unit");
  });
});

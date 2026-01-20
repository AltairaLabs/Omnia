import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useAgentCost } from "./use-agent-cost";

// Mock prometheus utilities
const mockQueryPrometheus = vi.fn();
const mockQueryPrometheusRange = vi.fn();
const mockIsPrometheusAvailable = vi.fn();

vi.mock("@/lib/prometheus", () => ({
  queryPrometheus: (...args: unknown[]) => mockQueryPrometheus(...args),
  queryPrometheusRange: (...args: unknown[]) => mockQueryPrometheusRange(...args),
  isPrometheusAvailable: () => mockIsPrometheusAvailable(),
}));

vi.mock("@/lib/prometheus-queries", () => ({
  LLM_METRICS: {
    COST_USD: "llm_cost_usd_total",
    INPUT_TOKENS: "llm_input_tokens_total",
    OUTPUT_TOKENS: "llm_output_tokens_total",
    REQUESTS_TOTAL: "llm_requests_total",
  },
  LABELS: {
    AGENT: "agent",
    NAMESPACE: "namespace",
  },
}));

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

describe("useAgentCost", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockIsPrometheusAvailable.mockResolvedValue(true);
  });

  it("returns empty data when Prometheus is not available", async () => {
    mockIsPrometheusAvailable.mockResolvedValue(false);

    const { result } = renderHook(
      () => useAgentCost("test-namespace", "test-agent"),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.available).toBe(false);
    expect(result.current.data?.totalCost).toBe(0);
  });

  it("fetches cost data from Prometheus", async () => {
    mockQueryPrometheus
      .mockResolvedValueOnce({
        status: "success",
        data: { result: [{ value: [Date.now() / 1000, "12.50"] }] },
      })
      .mockResolvedValueOnce({
        status: "success",
        data: { result: [{ value: [Date.now() / 1000, "5000"] }] },
      })
      .mockResolvedValueOnce({
        status: "success",
        data: { result: [{ value: [Date.now() / 1000, "2500"] }] },
      })
      .mockResolvedValueOnce({
        status: "success",
        data: { result: [{ value: [Date.now() / 1000, "100"] }] },
      });

    mockQueryPrometheusRange.mockResolvedValue({
      status: "success",
      data: { result: [] },
    });

    const { result } = renderHook(
      () => useAgentCost("test-namespace", "test-agent"),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.available).toBe(true);
    expect(result.current.data?.totalCost).toBe(12.5);
    expect(result.current.data?.inputTokens).toBe(5000);
    expect(result.current.data?.outputTokens).toBe(2500);
    expect(result.current.data?.requests).toBe(100);
  });

  it("handles empty Prometheus results", async () => {
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: { result: [] },
    });

    mockQueryPrometheusRange.mockResolvedValue({
      status: "success",
      data: { result: [] },
    });

    const { result } = renderHook(
      () => useAgentCost("test-namespace", "test-agent"),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.available).toBe(true);
    expect(result.current.data?.totalCost).toBe(0);
    expect(result.current.data?.timeSeries).toEqual([]);
  });

  it("handles Prometheus query errors gracefully", async () => {
    mockQueryPrometheus.mockRejectedValue(new Error("Query failed"));
    mockQueryPrometheusRange.mockRejectedValue(new Error("Query failed"));

    const { result } = renderHook(
      () => useAgentCost("test-namespace", "test-agent"),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // Should return empty data on error, not throw
    expect(result.current.data?.available).toBe(false);
    expect(result.current.data?.totalCost).toBe(0);
  });

  it("does not fetch when namespace is empty", async () => {
    const { result } = renderHook(() => useAgentCost("", "test-agent"), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(mockIsPrometheusAvailable).not.toHaveBeenCalled();
  });

  it("does not fetch when agent name is empty", async () => {
    const { result } = renderHook(() => useAgentCost("test-namespace", ""), {
      wrapper: createWrapper(),
    });

    expect(result.current.fetchStatus).toBe("idle");
    expect(mockIsPrometheusAvailable).not.toHaveBeenCalled();
  });

  it("converts time series data to sparkline format", async () => {
    const now = Date.now() / 1000;
    const hourInSeconds = 3600;

    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: { result: [] },
    });

    mockQueryPrometheusRange.mockResolvedValue({
      status: "success",
      data: {
        result: [
          {
            metric: {},
            values: [
              [now - hourInSeconds * 2, "1.5"],
              [now - hourInSeconds, "2.0"],
              [now, "3.5"],
            ],
          },
        ],
      },
    });

    const { result } = renderHook(
      () => useAgentCost("test-namespace", "test-agent"),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.timeSeries).toBeDefined();
    // Should have 24 data points (hourly buckets for 24 hours)
    expect(result.current.data?.timeSeries.length).toBe(24);
  });

  it("includes correct query key for caching", async () => {
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: { result: [] },
    });

    mockQueryPrometheusRange.mockResolvedValue({
      status: "success",
      data: { result: [] },
    });

    const { result } = renderHook(
      () => useAgentCost("ns-1", "agent-1"),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    // Different namespace/agent should create different query
    expect(mockIsPrometheusAvailable).toHaveBeenCalled();
  });

  it("handles malformed Prometheus response", async () => {
    mockQueryPrometheus.mockResolvedValue({
      status: "error",
      data: null,
    });

    mockQueryPrometheusRange.mockResolvedValue({
      status: "error",
      data: null,
    });

    const { result } = renderHook(
      () => useAgentCost("test-namespace", "test-agent"),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(result.current.data?.available).toBe(true);
    expect(result.current.data?.totalCost).toBe(0);
  });
});

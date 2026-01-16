import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useProviderMetrics } from "./use-provider-metrics";

// Mock prometheus module
const mockIsPrometheusAvailable = vi.fn();
const mockQueryPrometheus = vi.fn();
const mockQueryPrometheusRange = vi.fn();

vi.mock("@/lib/prometheus", () => ({
  isPrometheusAvailable: () => mockIsPrometheusAvailable(),
  queryPrometheus: (...args: unknown[]) => mockQueryPrometheus(...args),
  queryPrometheusRange: (...args: unknown[]) => mockQueryPrometheusRange(...args),
}));

vi.mock("@/lib/prometheus-queries", () => ({
  LLMQueries: {
    requestRate: (filter: { provider: string }, interval: string) =>
      `sum(rate(requests{provider="${filter.provider}"}[${interval}]))`,
    inputTokenRate: (filter: { provider: string }, interval: string) =>
      `sum(rate(input_tokens{provider="${filter.provider}"}[${interval}]))`,
    outputTokenRate: (filter: { provider: string }, interval: string) =>
      `sum(rate(output_tokens{provider="${filter.provider}"}[${interval}]))`,
    costIncrease: (filter: { provider: string }, interval: string) =>
      `sum(increase(cost{provider="${filter.provider}"}[${interval}]))`,
  },
  LLM_METRICS: {
    REQUESTS_TOTAL: "omnia_llm_requests_total",
    INPUT_TOKENS: "omnia_llm_input_tokens_total",
    OUTPUT_TOKENS: "omnia_llm_output_tokens_total",
    COST_USD: "omnia_llm_cost_usd_total",
  },
  buildFilter: (filter: { provider: string }) => `provider="${filter.provider}"`,
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

// Helper to create mock Prometheus range response
function createRangeResponse(values: Array<[number, string]>) {
  return {
    status: "success",
    data: {
      result: [
        {
          metric: { provider: "anthropic" },
          values,
        },
      ],
    },
  };
}

// Helper to create mock Prometheus instant response
function createInstantResponse(value: string) {
  return {
    status: "success",
    data: {
      result: [
        {
          metric: { provider: "anthropic" },
          value: [Date.now() / 1000, value],
        },
      ],
    },
  };
}

describe("useProviderMetrics", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should return empty metrics when providerType is undefined", async () => {
    const { result } = renderHook(
      () => useProviderMetrics("test-provider", undefined),
      { wrapper: TestWrapper }
    );

    // Query should not be enabled
    expect(result.current.isLoading).toBe(false);
    expect(result.current.data).toBeUndefined();
  });

  it("should return empty metrics when Prometheus is unavailable", async () => {
    mockIsPrometheusAvailable.mockResolvedValue(false);

    const { result } = renderHook(
      () => useProviderMetrics("test-provider", "anthropic"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.available).toBe(false);
    expect(result.current.data?.requestRate).toEqual([]);
    expect(result.current.data?.totalRequests24h).toBe(0);
  });

  it("should fetch and process metrics successfully", async () => {
    mockIsPrometheusAvailable.mockResolvedValue(true);

    const now = Date.now() / 1000;
    const rangeValues: Array<[number, string]> = [
      [now - 3600, "10"],
      [now - 1800, "15"],
      [now, "20"],
    ];

    mockQueryPrometheusRange.mockResolvedValue(createRangeResponse(rangeValues));
    mockQueryPrometheus.mockImplementation((query: string) => {
      if (query.includes("requests")) {
        return Promise.resolve(createInstantResponse("100"));
      }
      if (query.includes("input_tokens")) {
        return Promise.resolve(createInstantResponse("50000"));
      }
      if (query.includes("output_tokens")) {
        return Promise.resolve(createInstantResponse("25000"));
      }
      if (query.includes("cost")) {
        return Promise.resolve(createInstantResponse("1.50"));
      }
      return Promise.resolve(createInstantResponse("0"));
    });

    const { result } = renderHook(
      () => useProviderMetrics("anthropic-provider", "anthropic"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.available).toBe(true);
    expect(result.current.data?.requestRate).toHaveLength(3);
    expect(result.current.data?.currentRequestRate).toBe(20);
    expect(result.current.data?.totalRequests24h).toBe(100);
    expect(result.current.data?.totalTokens24h).toBe(75000); // 50000 + 25000
    expect(result.current.data?.totalCost24h).toBe(1.5);
  });

  it("should handle empty Prometheus responses", async () => {
    mockIsPrometheusAvailable.mockResolvedValue(true);
    mockQueryPrometheusRange.mockResolvedValue({
      status: "success",
      data: { result: [] },
    });
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: { result: [] },
    });

    const { result } = renderHook(
      () => useProviderMetrics("test-provider", "openai"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.available).toBe(true);
    expect(result.current.data?.requestRate).toEqual([]);
    expect(result.current.data?.totalRequests24h).toBe(0);
  });

  it("should handle query errors gracefully", async () => {
    mockIsPrometheusAvailable.mockResolvedValue(true);
    mockQueryPrometheusRange.mockRejectedValue(new Error("Query failed"));
    mockQueryPrometheus.mockRejectedValue(new Error("Query failed"));

    const { result } = renderHook(
      () => useProviderMetrics("test-provider", "anthropic"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // Should return empty metrics on error (caught errors)
    expect(result.current.data?.available).toBe(true);
    expect(result.current.data?.requestRate).toEqual([]);
  });

  it("should handle malformed Prometheus responses", async () => {
    mockIsPrometheusAvailable.mockResolvedValue(true);
    mockQueryPrometheusRange.mockResolvedValue({
      status: "error",
      data: null,
    });
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: { result: [{ value: null }] },
    });

    const { result } = renderHook(
      () => useProviderMetrics("test-provider", "anthropic"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.requestRate).toEqual([]);
    expect(result.current.data?.totalRequests24h).toBe(0);
  });

  it("should use correct query key", async () => {
    mockIsPrometheusAvailable.mockResolvedValue(true);
    mockQueryPrometheusRange.mockResolvedValue(createRangeResponse([]));
    mockQueryPrometheus.mockResolvedValue(createInstantResponse("0"));

    const { result } = renderHook(
      () => useProviderMetrics("my-provider", "ollama"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    // Verify the queries were made with correct filter
    expect(mockQueryPrometheus).toHaveBeenCalledWith(
      expect.stringContaining('provider="ollama"')
    );
  });

  it("should convert timestamps correctly in sparkline data", async () => {
    mockIsPrometheusAvailable.mockResolvedValue(true);

    const timestamp = 1700000000; // Unix timestamp in seconds
    mockQueryPrometheusRange.mockResolvedValue(
      createRangeResponse([[timestamp, "42"]])
    );
    mockQueryPrometheus.mockResolvedValue(createInstantResponse("0"));

    const { result } = renderHook(
      () => useProviderMetrics("test-provider", "anthropic"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data?.requestRate[0].timestamp).toEqual(
      new Date(timestamp * 1000)
    );
    expect(result.current.data?.requestRate[0].value).toBe(42);
  });
});

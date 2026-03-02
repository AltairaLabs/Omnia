/**
 * Tests for useEvalPassRateTrends and useEvalMetrics hooks.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

// Mock dependencies
const mockQueryPrometheus = vi.fn();
const mockQueryPrometheusRange = vi.fn();
const mockQueryPrometheusMetadata = vi.fn();

vi.mock("@/lib/prometheus", () => ({
  queryPrometheus: (...args: unknown[]) => mockQueryPrometheus(...args),
  queryPrometheusRange: (...args: unknown[]) => mockQueryPrometheusRange(...args),
  queryPrometheusMetadata: (...args: unknown[]) => mockQueryPrometheusMetadata(...args),
}));

vi.mock("@/lib/prometheus-queries", () => ({
  EvalQueries: {
    discoverMetrics: () => '{__name__=~"omnia_eval_.*"}',
    metricAvgOverTime: (name: string, window: string) =>
      `avg_over_time(${name}[${window}])`,
    metricValue: (name: string) => name,
  },
}));

import { useEvalPassRateTrends, useEvalMetrics, EVAL_TREND_RANGES } from "./use-eval-trends";

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function TestQueryProvider({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  }
  return TestQueryProvider;
}

describe("EVAL_TREND_RANGES", () => {
  it("defines expected time ranges", () => {
    expect(EVAL_TREND_RANGES).toHaveProperty("1h");
    expect(EVAL_TREND_RANGES).toHaveProperty("6h");
    expect(EVAL_TREND_RANGES).toHaveProperty("24h");
    expect(EVAL_TREND_RANGES).toHaveProperty("7d");
    expect(EVAL_TREND_RANGES).toHaveProperty("30d");
    expect(EVAL_TREND_RANGES["1h"].seconds).toBe(3600);
    expect(EVAL_TREND_RANGES["1h"].step).toBe("1m");
  });
});

describe("useEvalPassRateTrends", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns empty array when no metrics are discovered", async () => {
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: { result: [] },
    });

    const { result } = renderHook(
      () => useEvalPassRateTrends({ timeRange: "1h" }),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it("returns empty array when Prometheus returns no data for discovery", async () => {
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: { result: [] },
    });

    const { result } = renderHook(
      () => useEvalPassRateTrends(),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it("merges time series data correctly from multiple metrics", async () => {
    // Discovery returns two metrics
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [
          { metric: { __name__: "omnia_eval_tone" }, value: [1000, "0.9"] },
          { metric: { __name__: "omnia_eval_safety" }, value: [1000, "0.8"] },
        ],
      },
    });

    // Range query returns time series for each metric
    mockQueryPrometheusRange
      .mockResolvedValueOnce({
        status: "success",
        data: {
          result: [
            {
              metric: { __name__: "omnia_eval_tone" },
              values: [
                [1000, "0.85"],
                [1060, "0.90"],
              ],
            },
          ],
        },
      })
      .mockResolvedValueOnce({
        status: "success",
        data: {
          result: [
            {
              metric: { __name__: "omnia_eval_safety" },
              values: [
                [1000, "0.95"],
                [1060, "0.80"],
              ],
            },
          ],
        },
      });

    const { result } = renderHook(
      () =>
        useEvalPassRateTrends({
          metricNames: ["omnia_eval_tone", "omnia_eval_safety"],
          timeRange: "1h",
        }),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const data = result.current.data!;
    expect(data).toHaveLength(2);
    // Timestamps are sorted
    expect(data[0].timestamp).toEqual(new Date(1000 * 1000));
    expect(data[1].timestamp).toEqual(new Date(1060 * 1000));
    // Values merged from both metrics (prefix stripped)
    expect(data[0].values).toEqual({ tone: 0.85, safety: 0.95 });
    expect(data[1].values).toEqual({ tone: 0.9, safety: 0.8 });
  });

  it("skips metrics with failed status in range response", async () => {
    mockQueryPrometheusRange
      .mockResolvedValueOnce({
        status: "success",
        data: {
          result: [
            {
              metric: { __name__: "omnia_eval_tone" },
              values: [[1000, "0.9"]],
            },
          ],
        },
      })
      .mockResolvedValueOnce({
        status: "error",
        error: "bad query",
      });

    const { result } = renderHook(
      () =>
        useEvalPassRateTrends({
          metricNames: ["omnia_eval_tone", "omnia_eval_bad"],
          timeRange: "1h",
        }),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const data = result.current.data!;
    expect(data).toHaveLength(1);
    expect(data[0].values).toEqual({ tone: 0.9 });
  });

  it("handles Prometheus discovery errors gracefully (returns empty)", async () => {
    // discoverEvalMetrics catches errors internally and returns []
    mockQueryPrometheus.mockRejectedValue(new Error("Network error"));

    const { result } = renderHook(
      () => useEvalPassRateTrends({ timeRange: "1h" }),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it("propagates range query errors with retry: false", async () => {
    // When metricNames are provided, discovery is skipped, so range query errors propagate
    mockQueryPrometheusRange.mockRejectedValue(new Error("Range query failed"));

    const { result } = renderHook(
      () =>
        useEvalPassRateTrends({
          metricNames: ["omnia_eval_tone"],
          timeRange: "1h",
        }),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeDefined();
    // Should not retry (retry: false in the hook)
    expect(mockQueryPrometheusRange).toHaveBeenCalledTimes(1);
  });

  it("filters out infrastructure suffixes during discovery", async () => {
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [
          { metric: { __name__: "omnia_eval_latency" }, value: [1000, "1"] },
          { metric: { __name__: "omnia_eval_latency_bucket" }, value: [1000, "1"] },
          { metric: { __name__: "omnia_eval_latency_sum" }, value: [1000, "1"] },
          { metric: { __name__: "omnia_eval_latency_count" }, value: [1000, "1"] },
          { metric: { __name__: "omnia_eval_executed_total" }, value: [1000, "47"] },
          { metric: { __name__: "omnia_eval_passed_total" }, value: [1000, "42"] },
        ],
      },
    });

    mockQueryPrometheusRange.mockResolvedValue({
      status: "success",
      data: {
        result: [
          {
            metric: { __name__: "omnia_eval_latency" },
            values: [[1000, "0.5"]],
          },
        ],
      },
    });

    const { result } = renderHook(
      () => useEvalPassRateTrends({ timeRange: "1h" }),
      { wrapper: createWrapper() }
    );

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    // Only one range query for the non-suffixed metric
    expect(mockQueryPrometheusRange).toHaveBeenCalledTimes(1);
  });
});

describe("useEvalMetrics", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Default metadata mock returns gauge for all metrics
    mockQueryPrometheusMetadata.mockResolvedValue({
      omnia_eval_safety: "gauge",
      omnia_eval_tone: "gauge",
    });
  });

  it("discovers and returns metrics with values and types", async () => {
    mockQueryPrometheusMetadata.mockResolvedValue({
      omnia_eval_safety: "gauge",
      omnia_eval_tone: "counter",
    });
    // Discovery call returns sorted names: safety, tone
    mockQueryPrometheus
      .mockResolvedValueOnce({
        status: "success",
        data: {
          result: [
            { metric: { __name__: "omnia_eval_tone" }, value: [1000, "0.9"] },
            { metric: { __name__: "omnia_eval_safety" }, value: [1000, "0.8"] },
          ],
        },
      })
      // Individual metric value calls (alphabetical: safety first, then tone)
      .mockResolvedValueOnce({
        status: "success",
        data: { result: [{ metric: {}, value: [1000, "0.78"] }] },
      })
      .mockResolvedValueOnce({
        status: "success",
        data: { result: [{ metric: {}, value: [1000, "0.92"] }] },
      });

    const { result } = renderHook(() => useEvalMetrics(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const metrics = result.current.data!;
    expect(metrics).toHaveLength(2);
    expect(metrics[0]).toEqual({ name: "omnia_eval_safety", value: 0.78, metricType: "gauge" });
    expect(metrics[1]).toEqual({ name: "omnia_eval_tone", value: 0.92, metricType: "counter" });
  });

  it("returns empty array when no metrics discovered", async () => {
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: { result: [] },
    });

    const { result } = renderHook(() => useEvalMetrics(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it("returns value 0 when Prometheus returns error for individual metric", async () => {
    mockQueryPrometheus
      .mockResolvedValueOnce({
        status: "success",
        data: {
          result: [
            { metric: { __name__: "omnia_eval_tone" }, value: [1000, "0.9"] },
          ],
        },
      })
      .mockResolvedValueOnce({
        status: "error",
        error: "bad query",
      });

    const { result } = renderHook(() => useEvalMetrics(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([{ name: "omnia_eval_tone", value: 0, metricType: "gauge" }]);
  });

  it("handles discovery failure gracefully by returning empty", async () => {
    // discoverEvalMetrics catches errors and returns [], so the queryFn
    // succeeds with an empty array rather than throwing.
    mockQueryPrometheus.mockRejectedValue(new Error("Network error"));

    const { result } = renderHook(() => useEvalMetrics(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it("returns 0 when metric value is missing from result", async () => {
    mockQueryPrometheus
      .mockResolvedValueOnce({
        status: "success",
        data: {
          result: [
            { metric: { __name__: "omnia_eval_tone" }, value: [1000, "0.9"] },
          ],
        },
      })
      .mockResolvedValueOnce({
        status: "success",
        data: { result: [] },
      });

    const { result } = renderHook(() => useEvalMetrics(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([{ name: "omnia_eval_tone", value: 0, metricType: "gauge" }]);
  });

  it("defaults to gauge when metadata fetch fails", async () => {
    mockQueryPrometheusMetadata.mockRejectedValue(new Error("Metadata error"));
    mockQueryPrometheus
      .mockResolvedValueOnce({
        status: "success",
        data: {
          result: [
            { metric: { __name__: "omnia_eval_tone" }, value: [1000, "0.9"] },
          ],
        },
      })
      .mockResolvedValueOnce({
        status: "success",
        data: { result: [{ metric: {}, value: [1000, "0.85"] }] },
      });

    const { result } = renderHook(() => useEvalMetrics(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([{ name: "omnia_eval_tone", value: 0.85, metricType: "gauge" }]);
  });
});

/**
 * Tests for eval quality hooks.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { renderHook, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

const mockQueryPrometheus = vi.fn();

vi.mock("@/lib/prometheus", () => ({
  queryPrometheus: (...args: unknown[]) => mockQueryPrometheus(...args),
}));

vi.mock("@/lib/prometheus-queries", () => ({
  EvalQueries: {
    discoverMetrics: () => '{__name__=~"omnia_eval_.*"}',
  },
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: { name: "test-workspace" },
  }),
}));

import { useEvalSummary, useRecentEvalFailures } from "./use-eval-quality";

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function TestQueryProvider({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  }
  return TestQueryProvider;
}

describe("useEvalSummary", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("discovers metrics and returns summaries from Prometheus", async () => {
    // Discovery call returns two metrics
    mockQueryPrometheus
      .mockResolvedValueOnce({
        status: "success",
        data: {
          result: [
            { metric: { __name__: "omnia_eval_safety" }, value: [1000, "0.8"] },
            { metric: { __name__: "omnia_eval_tone" }, value: [1000, "0.9"] },
          ],
        },
      })
      // Individual metric value calls (alphabetical: safety first, then tone)
      .mockResolvedValueOnce({
        status: "success",
        data: { result: [{ metric: {}, value: [1000, "0.96"] }] },
      })
      .mockResolvedValueOnce({
        status: "success",
        data: { result: [{ metric: {}, value: [1000, "0.85"] }] },
      });

    const { result } = renderHook(() => useEvalSummary(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const data = result.current.data!;
    expect(data).toHaveLength(2);
    // Sorted alphabetically: safety first
    expect(data[0].evalId).toBe("safety");
    expect(data[0].evalType).toBe("metric");
    expect(data[0].passRate).toBeCloseTo(96.0);
    expect(data[0].avgScore).toBeCloseTo(0.96);
    expect(data[0].total).toBe(0);
    expect(data[0].passed).toBe(0);
    expect(data[0].failed).toBe(0);

    expect(data[1].evalId).toBe("tone");
    expect(data[1].passRate).toBeCloseTo(85.0);
    expect(data[1].avgScore).toBeCloseTo(0.85);
  });

  it("returns empty array when no metrics are discovered", async () => {
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: { result: [] },
    });

    const { result } = renderHook(() => useEvalSummary(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([]);
  });

  it("handles discovery failure gracefully", async () => {
    mockQueryPrometheus.mockRejectedValue(new Error("Network error"));

    const { result } = renderHook(() => useEvalSummary(), {
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

    const { result } = renderHook(() => useEvalSummary(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const data = result.current.data!;
    expect(data).toHaveLength(1);
    expect(data[0].evalId).toBe("tone");
    expect(data[0].passRate).toBe(0);
    expect(data[0].avgScore).toBe(0);
  });

  it("filters out _bucket, _sum, _count metrics during discovery", async () => {
    mockQueryPrometheus
      .mockResolvedValueOnce({
        status: "success",
        data: {
          result: [
            { metric: { __name__: "omnia_eval_latency" }, value: [1000, "1"] },
            { metric: { __name__: "omnia_eval_latency_bucket" }, value: [1000, "1"] },
            { metric: { __name__: "omnia_eval_latency_sum" }, value: [1000, "1"] },
            { metric: { __name__: "omnia_eval_latency_count" }, value: [1000, "1"] },
          ],
        },
      })
      // Only one individual metric query for the non-suffixed metric
      .mockResolvedValueOnce({
        status: "success",
        data: { result: [{ metric: {}, value: [1000, "0.5"] }] },
      });

    const { result } = renderHook(() => useEvalSummary(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(1);
    expect(result.current.data![0].evalId).toBe("latency");
    // Discovery + 1 individual metric query = 2 total calls
    expect(mockQueryPrometheus).toHaveBeenCalledTimes(2);
  });
});

describe("useRecentEvalFailures", () => {
  it("returns empty data (session-api has no eval-results endpoint yet)", async () => {
    const { result } = renderHook(() => useRecentEvalFailures(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({ evalResults: [], total: 0 });
  });
});

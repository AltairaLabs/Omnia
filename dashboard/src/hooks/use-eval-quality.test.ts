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
const mockQueryPrometheusMetadata = vi.fn();

vi.mock("@/lib/prometheus", () => ({
  queryPrometheus: (...args: unknown[]) => mockQueryPrometheus(...args),
  queryPrometheusMetadata: (...args: unknown[]) => mockQueryPrometheusMetadata(...args),
}));

vi.mock("@/lib/prometheus-queries", () => ({
  EvalQueries: {
    discoverMetrics: () => '{__name__=~"omnia_eval_.*"}',
    metricValue: (name: string) => name,
  },
}));

import { useEvalSummary } from "./use-eval-quality";

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
    mockQueryPrometheusMetadata.mockResolvedValue({
      omnia_eval_safety: "gauge",
      omnia_eval_tone: "gauge",
    });
  });

  it("discovers metrics and returns score summaries from Prometheus", async () => {
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
    expect(data[0].evalId).toBe("safety");
    expect(data[0].score).toBeCloseTo(0.96);
    expect(data[0].metricType).toBe("gauge");

    expect(data[1].evalId).toBe("tone");
    expect(data[1].score).toBeCloseTo(0.85);
    expect(data[1].metricType).toBe("gauge");
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

  it("returns score 0 when Prometheus returns error for individual metric", async () => {
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
    expect(data[0].score).toBe(0);
    expect(data[0].metricType).toBe("gauge");
  });

  it("filters out histogram sub-metrics using metadata, not suffix heuristics", async () => {
    // Metadata: latency is a histogram, so _bucket/_sum/_count are sub-metrics
    mockQueryPrometheusMetadata.mockResolvedValue({
      omnia_eval_latency: "histogram",
    });
    mockQueryPrometheus
      .mockResolvedValueOnce({
        status: "success",
        data: {
          result: [
            { metric: { __name__: "omnia_eval_latency" }, value: [1000, "1"] },
            { metric: { __name__: "omnia_eval_latency_bucket" }, value: [1000, "1"] },
            { metric: { __name__: "omnia_eval_latency_sum" }, value: [1000, "1"] },
            { metric: { __name__: "omnia_eval_latency_count" }, value: [1000, "1"] },
            { metric: { __name__: "omnia_eval_executed_total" }, value: [1000, "47"] },
            { metric: { __name__: "omnia_eval_passed_total" }, value: [1000, "42"] },
            { metric: { __name__: "omnia_eval_failed_total" }, value: [1000, "5"] },
            { metric: { __name__: "omnia_eval_score" }, value: [1000, "0.9"] },
            { metric: { __name__: "omnia_eval_duration_seconds" }, value: [1000, "1.2"] },
            { metric: { __name__: "omnia_eval_worker_events_received_total" }, value: [1000, "99"] },
          ],
        },
      })
      // Only latency survives filtering
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
    expect(result.current.data![0].metricType).toBe("histogram");
    // Discovery + 1 individual metric query = 2 calls
    expect(mockQueryPrometheus).toHaveBeenCalledTimes(2);
  });

  it("does NOT filter eval names that happen to end with histogram suffixes", async () => {
    // word_count and response_created are gauges — not histogram sub-metrics
    mockQueryPrometheusMetadata.mockResolvedValue({
      omnia_eval_word_count: "gauge",
      omnia_eval_response_created: "gauge",
    });
    mockQueryPrometheus
      .mockResolvedValueOnce({
        status: "success",
        data: {
          result: [
            { metric: { __name__: "omnia_eval_word_count" }, value: [1000, "0.8"] },
            { metric: { __name__: "omnia_eval_response_created" }, value: [1000, "0.7"] },
          ],
        },
      })
      .mockResolvedValueOnce({
        status: "success",
        data: { result: [{ metric: {}, value: [1000, "0.8"] }] },
      })
      .mockResolvedValueOnce({
        status: "success",
        data: { result: [{ metric: {}, value: [1000, "0.7"] }] },
      });

    const { result } = renderHook(() => useEvalSummary(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(2);
    const evalIds = result.current.data!.map((d) => d.evalId);
    expect(evalIds).toContain("word_count");
    expect(evalIds).toContain("response_created");
  });

  it("builds histogram summary with score", async () => {
    mockQueryPrometheusMetadata.mockResolvedValue({
      omnia_eval_duration: "histogram",
    });
    mockQueryPrometheus
      .mockResolvedValueOnce({
        status: "success",
        data: {
          result: [
            { metric: { __name__: "omnia_eval_duration" }, value: [1000, "1.5"] },
          ],
        },
      })
      .mockResolvedValueOnce({
        status: "success",
        data: { result: [{ metric: {}, value: [1000, "1.5"] }] },
      });

    const { result } = renderHook(() => useEvalSummary(), {
      wrapper: createWrapper(),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const data = result.current.data!;
    expect(data).toHaveLength(1);
    expect(data[0].metricType).toBe("histogram");
    expect(data[0].score).toBe(1.5);
  });
});

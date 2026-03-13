/**
 * Tests for eval metric discovery and filtering.
 *
 * This is the critical filtering logic — tested directly to prevent
 * regressions where legitimate eval metrics get silently dropped.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";

const mockQueryPrometheus = vi.fn();
const mockQueryPrometheusMetadata = vi.fn();

vi.mock("@/lib/prometheus", () => ({
  queryPrometheus: (...args: unknown[]) => mockQueryPrometheus(...args),
  queryPrometheusMetadata: (...args: unknown[]) => mockQueryPrometheusMetadata(...args),
}));

vi.mock("@/lib/prometheus-queries", () => ({
  EvalQueries: {
    discoverMetrics: () => '{__name__=~"omnia_eval_.*"}',
  },
}));

import { discoverEvalMetrics, toEvalMetricType } from "./eval-discovery";

describe("toEvalMetricType", () => {
  it("maps counter to counter", () => {
    expect(toEvalMetricType("counter")).toBe("counter");
  });
  it("maps histogram to histogram", () => {
    expect(toEvalMetricType("histogram")).toBe("histogram");
  });
  it("maps gauge to gauge", () => {
    expect(toEvalMetricType("gauge")).toBe("gauge");
  });
  it("maps unknown to gauge", () => {
    expect(toEvalMetricType("unknown")).toBe("gauge");
  });
  it("maps summary to gauge", () => {
    expect(toEvalMetricType("summary")).toBe("gauge");
  });
});

describe("discoverEvalMetrics", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns discovered metrics sorted alphabetically with types", async () => {
    mockQueryPrometheusMetadata.mockResolvedValue({
      omnia_eval_tone: "gauge",
      omnia_eval_safety: "gauge",
    });
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [
          { metric: { __name__: "omnia_eval_tone" }, value: [1000, "0.9"] },
          { metric: { __name__: "omnia_eval_safety" }, value: [1000, "0.8"] },
        ],
      },
    });

    const result = await discoverEvalMetrics();
    expect(result).toEqual([
      { name: "omnia_eval_safety", metricType: "gauge" },
      { name: "omnia_eval_tone", metricType: "gauge" },
    ]);
  });

  it("returns empty array when Prometheus returns no data", async () => {
    mockQueryPrometheusMetadata.mockResolvedValue({});
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: { result: [] },
    });

    const result = await discoverEvalMetrics();
    expect(result).toEqual([]);
  });

  it("returns empty array on network error", async () => {
    mockQueryPrometheus.mockRejectedValue(new Error("Network error"));
    mockQueryPrometheusMetadata.mockRejectedValue(new Error("Network error"));

    const result = await discoverEvalMetrics();
    expect(result).toEqual([]);
  });

  // --- Infra metric exclusion ---

  it("excludes omnia_eval_worker_* prefix metrics", async () => {
    mockQueryPrometheusMetadata.mockResolvedValue({
      omnia_eval_worker_events_total: "counter",
    });
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [
          { metric: { __name__: "omnia_eval_worker_events_total" }, value: [1000, "99"] },
        ],
      },
    });

    const result = await discoverEvalMetrics();
    expect(result).toEqual([]);
  });

  it("excludes hard-coded infra metric names", async () => {
    mockQueryPrometheusMetadata.mockResolvedValue({
      omnia_eval_executed_total: "counter",
      omnia_eval_score: "gauge",
      omnia_eval_duration_seconds: "histogram",
      omnia_eval_passed_total: "counter",
      omnia_eval_failed_total: "counter",
    });
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [
          { metric: { __name__: "omnia_eval_executed_total" }, value: [1000, "47"] },
          { metric: { __name__: "omnia_eval_score" }, value: [1000, "0.9"] },
          { metric: { __name__: "omnia_eval_duration_seconds" }, value: [1000, "1.2"] },
          { metric: { __name__: "omnia_eval_passed_total" }, value: [1000, "42"] },
          { metric: { __name__: "omnia_eval_failed_total" }, value: [1000, "5"] },
        ],
      },
    });

    const result = await discoverEvalMetrics();
    expect(result).toEqual([]);
  });

  // --- Histogram sub-metric exclusion (metadata-driven) ---

  it("excludes histogram sub-metrics when base metric is histogram in metadata", async () => {
    mockQueryPrometheusMetadata.mockResolvedValue({
      omnia_eval_latency: "histogram",
    });
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [
          { metric: { __name__: "omnia_eval_latency" }, value: [1000, "1.5"] },
          { metric: { __name__: "omnia_eval_latency_bucket" }, value: [1000, "1"] },
          { metric: { __name__: "omnia_eval_latency_sum" }, value: [1000, "3.0"] },
          { metric: { __name__: "omnia_eval_latency_count" }, value: [1000, "2"] },
          { metric: { __name__: "omnia_eval_latency_created" }, value: [1000, "1000"] },
        ],
      },
    });

    const result = await discoverEvalMetrics();
    expect(result).toEqual([
      { name: "omnia_eval_latency", metricType: "histogram" },
    ]);
  });

  // --- False-positive prevention (the bug that prompted this refactor) ---

  it("KEEPS eval metrics whose names happen to end with _count", async () => {
    mockQueryPrometheusMetadata.mockResolvedValue({
      omnia_eval_word_count: "gauge",
    });
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [
          { metric: { __name__: "omnia_eval_word_count" }, value: [1000, "0.8"] },
        ],
      },
    });

    const result = await discoverEvalMetrics();
    expect(result).toEqual([
      { name: "omnia_eval_word_count", metricType: "gauge" },
    ]);
  });

  it("KEEPS eval metrics whose names happen to end with _sum", async () => {
    mockQueryPrometheusMetadata.mockResolvedValue({
      omnia_eval_check_sum: "gauge",
    });
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [
          { metric: { __name__: "omnia_eval_check_sum" }, value: [1000, "0.7"] },
        ],
      },
    });

    const result = await discoverEvalMetrics();
    expect(result).toEqual([
      { name: "omnia_eval_check_sum", metricType: "gauge" },
    ]);
  });

  it("KEEPS eval metrics whose names happen to end with _created", async () => {
    mockQueryPrometheusMetadata.mockResolvedValue({
      omnia_eval_response_created: "gauge",
    });
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [
          { metric: { __name__: "omnia_eval_response_created" }, value: [1000, "0.6"] },
        ],
      },
    });

    const result = await discoverEvalMetrics();
    expect(result).toEqual([
      { name: "omnia_eval_response_created", metricType: "gauge" },
    ]);
  });

  it("KEEPS eval metrics whose names happen to end with _bucket", async () => {
    mockQueryPrometheusMetadata.mockResolvedValue({
      omnia_eval_token_bucket: "gauge",
    });
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [
          { metric: { __name__: "omnia_eval_token_bucket" }, value: [1000, "0.5"] },
        ],
      },
    });

    const result = await discoverEvalMetrics();
    expect(result).toEqual([
      { name: "omnia_eval_token_bucket", metricType: "gauge" },
    ]);
  });

  // --- Mixed scenario ---

  it("correctly handles mix of real evals, histogram sub-metrics, and infra metrics", async () => {
    mockQueryPrometheusMetadata.mockResolvedValue({
      omnia_eval_session_helpfulness: "gauge",
      omnia_eval_tone: "gauge",
      omnia_eval_word_count: "gauge",
      omnia_eval_latency: "histogram",
      omnia_eval_executed_total: "counter",
    });
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [
          // Real evals — should survive
          { metric: { __name__: "omnia_eval_session_helpfulness" }, value: [1000, "0.9"] },
          { metric: { __name__: "omnia_eval_tone" }, value: [1000, "0.85"] },
          { metric: { __name__: "omnia_eval_word_count" }, value: [1000, "0.7"] },
          // Histogram base — should survive
          { metric: { __name__: "omnia_eval_latency" }, value: [1000, "1.5"] },
          // Histogram sub-metrics — should be excluded
          { metric: { __name__: "omnia_eval_latency_bucket" }, value: [1000, "1"] },
          { metric: { __name__: "omnia_eval_latency_sum" }, value: [1000, "3.0"] },
          { metric: { __name__: "omnia_eval_latency_count" }, value: [1000, "2"] },
          // Infra — should be excluded
          { metric: { __name__: "omnia_eval_executed_total" }, value: [1000, "47"] },
          { metric: { __name__: "omnia_eval_worker_events_total" }, value: [1000, "99"] },
        ],
      },
    });

    const result = await discoverEvalMetrics();
    expect(result).toEqual([
      { name: "omnia_eval_latency", metricType: "histogram" },
      { name: "omnia_eval_session_helpfulness", metricType: "gauge" },
      { name: "omnia_eval_tone", metricType: "gauge" },
      { name: "omnia_eval_word_count", metricType: "gauge" },
    ]);
  });

  // --- Metadata edge cases ---

  it("keeps metrics with no metadata entry as gauge (not excluded as sub-metric)", async () => {
    // Metric has no metadata entry AND no base histogram exists — keep it
    mockQueryPrometheusMetadata.mockResolvedValue({});
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [
          { metric: { __name__: "omnia_eval_new_metric" }, value: [1000, "0.5"] },
        ],
      },
    });

    const result = await discoverEvalMetrics();
    expect(result).toEqual([
      { name: "omnia_eval_new_metric", metricType: "gauge" },
    ]);
  });

  it("deduplicates metrics from Prometheus response", async () => {
    mockQueryPrometheusMetadata.mockResolvedValue({
      omnia_eval_tone: "gauge",
    });
    mockQueryPrometheus.mockResolvedValue({
      status: "success",
      data: {
        result: [
          { metric: { __name__: "omnia_eval_tone" }, value: [1000, "0.9"] },
          { metric: { __name__: "omnia_eval_tone" }, value: [1001, "0.8"] },
        ],
      },
    });

    const result = await discoverEvalMetrics();
    expect(result).toHaveLength(1);
  });
});

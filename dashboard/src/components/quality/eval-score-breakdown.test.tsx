/**
 * Tests for EvalScoreBreakdown component.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

// Mock hooks
const mockUseEvalMetrics = vi.fn();

vi.mock("@/hooks", () => ({
  useEvalMetrics: (...args: unknown[]) => mockUseEvalMetrics(...args),
}));

import {
  EvalScoreBreakdown,
  formatMetricName,
  getGaugeColor,
  getGaugeBarClass,
} from "./eval-score-breakdown";

const sampleSparkline = Array.from({ length: 10 }, (_, i) => ({ value: 0.8 + i * 0.02 }));

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function TestQueryProvider({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  }
  return TestQueryProvider;
}

describe("formatMetricName", () => {
  it("strips omnia_eval_ prefix and converts underscores to spaces", () => {
    expect(formatMetricName("omnia_eval_tone_quality")).toBe("tone quality");
  });

  it("handles metric without prefix", () => {
    expect(formatMetricName("custom_metric")).toBe("custom metric");
  });

  it("handles metric with only prefix", () => {
    expect(formatMetricName("omnia_eval_")).toBe("");
  });
});

describe("getGaugeColor", () => {
  it("returns green for high values", () => {
    expect(getGaugeColor(0.95)).toContain("green");
  });

  it("returns yellow for medium values", () => {
    expect(getGaugeColor(0.75)).toContain("yellow");
  });

  it("returns red for low values", () => {
    expect(getGaugeColor(0.5)).toContain("red");
  });
});

describe("getGaugeBarClass", () => {
  it("returns green class for high values", () => {
    expect(getGaugeBarClass(0.95)).toContain("green");
  });

  it("returns yellow class for medium values", () => {
    expect(getGaugeBarClass(0.75)).toContain("yellow");
  });

  it("returns red class for low values", () => {
    expect(getGaugeBarClass(0.5)).toContain("red");
  });
});

describe("EvalScoreBreakdown", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows skeleton when loading", () => {
    mockUseEvalMetrics.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    });

    const Wrapper = createWrapper();
    const { container } = render(
      <Wrapper>
        <EvalScoreBreakdown />
      </Wrapper>
    );

    const skeletons = container.querySelectorAll('[data-slot="skeleton"]');
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("shows 'No eval metrics found' when empty", () => {
    mockUseEvalMetrics.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <EvalScoreBreakdown />
      </Wrapper>
    );

    expect(screen.getByText("No eval metrics found")).toBeInTheDocument();
  });

  it("shows error message when query fails", () => {
    mockUseEvalMetrics.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("Prometheus error"),
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <EvalScoreBreakdown />
      </Wrapper>
    );

    expect(screen.getByText("Unable to load eval metrics from Prometheus")).toBeInTheDocument();
  });

  it("renders gauge metrics with progress bar, percentage, and sparkline", () => {
    mockUseEvalMetrics.mockReturnValue({
      data: [
        { name: "omnia_eval_tone", value: 0.95, metricType: "gauge", sparkline: sampleSparkline },
        { name: "omnia_eval_safety", value: 0.65, metricType: "gauge", sparkline: sampleSparkline },
      ],
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <EvalScoreBreakdown />
      </Wrapper>
    );

    expect(screen.getByText("tone")).toBeInTheDocument();
    expect(screen.getByText("safety")).toBeInTheDocument();
    expect(screen.getByText("95%")).toBeInTheDocument();
    expect(screen.getByText("65%")).toBeInTheDocument();
    expect(screen.getAllByTestId("gauge-display")).toHaveLength(2);
  });

  it("sorts metrics worst-first (ascending by value)", () => {
    mockUseEvalMetrics.mockReturnValue({
      data: [
        { name: "omnia_eval_tone", value: 0.95, metricType: "gauge", sparkline: sampleSparkline },
        { name: "omnia_eval_safety", value: 0.65, metricType: "gauge", sparkline: sampleSparkline },
      ],
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <EvalScoreBreakdown />
      </Wrapper>
    );

    const buttons = screen.getAllByRole("button");
    // safety (0.65) should come before tone (0.95)
    expect(buttons[0]).toHaveTextContent("safety");
    expect(buttons[1]).toHaveTextContent("tone");
  });

  it("renders counter metric as plain number", () => {
    mockUseEvalMetrics.mockReturnValue({
      data: [{ name: "omnia_eval_executed_total", value: 47, metricType: "counter", sparkline: sampleSparkline }],
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <EvalScoreBreakdown />
      </Wrapper>
    );

    expect(screen.getByTestId("counter-display")).toHaveTextContent("47");
  });

  it("renders histogram metric with seconds suffix", () => {
    mockUseEvalMetrics.mockReturnValue({
      data: [{ name: "omnia_eval_latency", value: 1.5, metricType: "histogram", sparkline: sampleSparkline }],
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <EvalScoreBreakdown />
      </Wrapper>
    );

    expect(screen.getByTestId("histogram-display")).toHaveTextContent("1.500s");
  });

  it("calls onSelectMetric when card is clicked", () => {
    const onSelectMetric = vi.fn();
    mockUseEvalMetrics.mockReturnValue({
      data: [{ name: "omnia_eval_tone", value: 0.95, metricType: "gauge", sparkline: sampleSparkline }],
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <EvalScoreBreakdown onSelectMetric={onSelectMetric} />
      </Wrapper>
    );

    fireEvent.click(screen.getByText("tone"));
    expect(onSelectMetric).toHaveBeenCalledWith("omnia_eval_tone");
  });

  it("highlights active metric card", () => {
    mockUseEvalMetrics.mockReturnValue({
      data: [
        { name: "omnia_eval_tone", value: 0.95, metricType: "gauge", sparkline: sampleSparkline },
        { name: "omnia_eval_safety", value: 0.8, metricType: "gauge", sparkline: sampleSparkline },
      ],
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <EvalScoreBreakdown activeMetric="omnia_eval_tone" />
      </Wrapper>
    );

    const toneCard = screen.getByText("tone").closest("[role='button']");
    const toneClasses = toneCard?.className.split(" ") ?? [];
    expect(toneClasses).toContain("bg-muted");

    const safetyCard = screen.getByText("safety").closest("[role='button']");
    const safetyClasses = safetyCard?.className.split(" ") ?? [];
    expect(safetyClasses).not.toContain("bg-muted");
  });

  it("passes filter to useEvalMetrics", () => {
    mockUseEvalMetrics.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
    });

    const filter = { agent: "chatbot" };

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <EvalScoreBreakdown filter={filter} />
      </Wrapper>
    );

    expect(mockUseEvalMetrics).toHaveBeenCalledWith(filter);
  });

  it("renders card header with title", () => {
    mockUseEvalMetrics.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <EvalScoreBreakdown />
      </Wrapper>
    );

    expect(screen.getByText("Eval Breakdown")).toBeInTheDocument();
  });
});

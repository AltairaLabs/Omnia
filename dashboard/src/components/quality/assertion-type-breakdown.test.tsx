/**
 * Tests for AssertionTypeBreakdown component.
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
  AssertionTypeBreakdown,
  formatMetricName,
  formatMetricValue,
  getMetricVariant,
  getMetricColor,
} from "./assertion-type-breakdown";

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

describe("getMetricVariant", () => {
  it("returns default for gauge metrics", () => {
    expect(getMetricVariant(0.95)).toBe("default");
    expect(getMetricVariant(0.5)).toBe("default");
  });

  it("returns outline for counter metrics", () => {
    expect(getMetricVariant(47, "counter")).toBe("outline");
  });

  it("returns outline for histogram metrics", () => {
    expect(getMetricVariant(1.5, "histogram")).toBe("outline");
  });
});

describe("getMetricColor", () => {
  it("returns foreground class for gauge metrics", () => {
    expect(getMetricColor(0.95)).toContain("text-foreground");
  });

  it("returns muted color for counter metrics", () => {
    expect(getMetricColor(47, "counter")).toContain("text-muted");
  });
});

describe("formatMetricValue", () => {
  it("formats gauge values as decimal", () => {
    expect(formatMetricValue(0.95, "gauge")).toBe("0.950");
  });

  it("formats counter values as rounded integers", () => {
    expect(formatMetricValue(47, "counter")).toBe("47");
  });

  it("formats histogram values with seconds suffix", () => {
    expect(formatMetricValue(1.5, "histogram")).toBe("1.500s");
  });
});

describe("AssertionTypeBreakdown", () => {
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
        <AssertionTypeBreakdown />
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
        <AssertionTypeBreakdown />
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
        <AssertionTypeBreakdown />
      </Wrapper>
    );

    expect(screen.getByText("Unable to load eval metrics from Prometheus")).toBeInTheDocument();
  });

  it("renders metric rows with name, value, and type badge", () => {
    mockUseEvalMetrics.mockReturnValue({
      data: [
        { name: "omnia_eval_tone", value: 0.95, metricType: "gauge" },
        { name: "omnia_eval_safety", value: 0.65, metricType: "gauge" },
      ],
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <AssertionTypeBreakdown />
      </Wrapper>
    );

    // Metric names with prefix stripped
    expect(screen.getByText("tone")).toBeInTheDocument();
    expect(screen.getByText("safety")).toBeInTheDocument();
    // Values
    expect(screen.getByText("0.950")).toBeInTheDocument();
    expect(screen.getByText("0.650")).toBeInTheDocument();
    // Type badges
    const gaugeBadges = screen.getAllByText("gauge");
    expect(gaugeBadges.length).toBe(2);
  });

  it("renders counter metric with type label instead of pass/fail", () => {
    mockUseEvalMetrics.mockReturnValue({
      data: [{ name: "omnia_eval_executed_total", value: 47, metricType: "counter" }],
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <AssertionTypeBreakdown />
      </Wrapper>
    );

    expect(screen.getByText("47")).toBeInTheDocument();
    expect(screen.getByText("counter")).toBeInTheDocument();
  });

  it("calls onSelectMetric when row is clicked", () => {
    const onSelectMetric = vi.fn();
    mockUseEvalMetrics.mockReturnValue({
      data: [{ name: "omnia_eval_tone", value: 0.95, metricType: "gauge" }],
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <AssertionTypeBreakdown onSelectMetric={onSelectMetric} />
      </Wrapper>
    );

    fireEvent.click(screen.getByText("tone"));
    expect(onSelectMetric).toHaveBeenCalledWith("omnia_eval_tone");
  });

  it("highlights active metric row", () => {
    mockUseEvalMetrics.mockReturnValue({
      data: [
        { name: "omnia_eval_tone", value: 0.95, metricType: "gauge" },
        { name: "omnia_eval_safety", value: 0.8, metricType: "gauge" },
      ],
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <AssertionTypeBreakdown activeMetric="omnia_eval_tone" />
      </Wrapper>
    );

    // The active row should have the standalone "bg-muted" class (not hover:bg-muted/50)
    const toneRow = screen.getByText("tone").closest("tr");
    const toneClasses = toneRow?.className.split(" ") ?? [];
    expect(toneClasses).toContain("bg-muted");

    // The non-active row should not have "bg-muted" as a standalone class
    const safetyRow = screen.getByText("safety").closest("tr");
    const safetyClasses = safetyRow?.className.split(" ") ?? [];
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
        <AssertionTypeBreakdown filter={filter} />
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
        <AssertionTypeBreakdown />
      </Wrapper>
    );

    expect(screen.getByText("Eval Metrics Breakdown")).toBeInTheDocument();
  });
});

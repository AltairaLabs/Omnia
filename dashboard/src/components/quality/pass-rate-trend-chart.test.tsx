/**
 * Tests for PassRateTrendChart component.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

// Mock hooks
const mockUseEvalPassRateTrends = vi.fn();

vi.mock("@/hooks", () => ({
  useEvalPassRateTrends: (...args: unknown[]) => mockUseEvalPassRateTrends(...args),
}));

// Mock recharts - render minimal DOM to test logic without SVG rendering
vi.mock("recharts", () => ({
  AreaChart: ({ children, data }: { children: React.ReactNode; data: unknown[] }) => (
    <div data-testid="area-chart" data-count={data.length}>
      {children}
    </div>
  ),
  Area: ({ dataKey }: { dataKey: string }) => (
    <div data-testid={`area-${dataKey}`} />
  ),
  XAxis: () => <div data-testid="x-axis" />,
  YAxis: () => <div data-testid="y-axis" />,
  CartesianGrid: () => <div data-testid="cartesian-grid" />,
  Tooltip: () => <div data-testid="tooltip" />,
  ResponsiveContainer: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="responsive-container">{children}</div>
  ),
  Legend: () => <div data-testid="legend" />,
}));

import { PassRateTrendChart, getEvalColor } from "./pass-rate-trend-chart";

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function TestQueryProvider({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  }
  return TestQueryProvider;
}

describe("getEvalColor", () => {
  it("returns colors from the palette", () => {
    expect(getEvalColor(0)).toBe("#2563eb");
    expect(getEvalColor(1)).toBe("#16a34a");
    expect(getEvalColor(2)).toBe("#d97706");
  });

  it("wraps around when index exceeds palette length", () => {
    expect(getEvalColor(10)).toBe(getEvalColor(0));
    expect(getEvalColor(11)).toBe(getEvalColor(1));
  });
});

describe("PassRateTrendChart", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows skeleton when loading", () => {
    mockUseEvalPassRateTrends.mockReturnValue({
      data: undefined,
      isLoading: true,
    });

    const Wrapper = createWrapper();
    const { container } = render(
      <Wrapper>
        <PassRateTrendChart timeRange="24h" />
      </Wrapper>
    );

    const skeletons = container.querySelectorAll('[data-slot="skeleton"]');
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("shows 'No trend data available' when no data", () => {
    mockUseEvalPassRateTrends.mockReturnValue({
      data: [],
      isLoading: false,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <PassRateTrendChart timeRange="24h" />
      </Wrapper>
    );

    expect(screen.getByText("No trend data available")).toBeInTheDocument();
  });

  it("shows 'No trend data available' when data is undefined", () => {
    mockUseEvalPassRateTrends.mockReturnValue({
      data: undefined,
      isLoading: false,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <PassRateTrendChart timeRange="24h" />
      </Wrapper>
    );

    expect(screen.getByText("No trend data available")).toBeInTheDocument();
  });

  it("renders chart when data is available", () => {
    mockUseEvalPassRateTrends.mockReturnValue({
      data: [
        {
          timestamp: new Date("2026-01-01T10:00:00Z"),
          values: { tone: 0.9, safety: 0.85 },
        },
        {
          timestamp: new Date("2026-01-01T11:00:00Z"),
          values: { tone: 0.92, safety: 0.88 },
        },
      ],
      isLoading: false,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <PassRateTrendChart timeRange="24h" />
      </Wrapper>
    );

    // Chart should be rendered
    expect(screen.getByTestId("area-chart")).toBeInTheDocument();
    expect(screen.getByTestId("responsive-container")).toBeInTheDocument();
    // One Area per series name (sorted alphabetically)
    expect(screen.getByTestId("area-safety")).toBeInTheDocument();
    expect(screen.getByTestId("area-tone")).toBeInTheDocument();
  });

  it("does not show chart or empty state while loading", () => {
    mockUseEvalPassRateTrends.mockReturnValue({
      data: undefined,
      isLoading: true,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <PassRateTrendChart timeRange="24h" />
      </Wrapper>
    );

    expect(screen.queryByTestId("area-chart")).not.toBeInTheDocument();
    expect(screen.queryByText("No trend data available")).not.toBeInTheDocument();
  });

  it("renders card header with title and description", () => {
    mockUseEvalPassRateTrends.mockReturnValue({
      data: [],
      isLoading: false,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <PassRateTrendChart timeRange="24h" />
      </Wrapper>
    );

    expect(screen.getByText("Eval Metric Trends")).toBeInTheDocument();
    expect(screen.getByText("Metric values over time from Prometheus")).toBeInTheDocument();
  });

  it("passes correct parameters to useEvalPassRateTrends", () => {
    mockUseEvalPassRateTrends.mockReturnValue({
      data: undefined,
      isLoading: true,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <PassRateTrendChart
          timeRange="7d"
          metricNames={["omnia_eval_tone"]}
        />
      </Wrapper>
    );

    expect(mockUseEvalPassRateTrends).toHaveBeenCalledWith({
      metricNames: ["omnia_eval_tone"],
      timeRange: "7d",
      filter: undefined,
    });
  });

  it("passes filter to useEvalPassRateTrends", () => {
    mockUseEvalPassRateTrends.mockReturnValue({
      data: [],
      isLoading: false,
    });

    const filter = { agent: "chatbot" };
    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <PassRateTrendChart timeRange="24h" filter={filter} />
      </Wrapper>
    );

    expect(mockUseEvalPassRateTrends).toHaveBeenCalledWith({
      metricNames: undefined,
      timeRange: "24h",
      filter,
    });
  });

  it("extracts and sorts unique series names from trend data", () => {
    mockUseEvalPassRateTrends.mockReturnValue({
      data: [
        {
          timestamp: new Date("2026-01-01T10:00:00Z"),
          values: { zeta: 0.5, alpha: 0.8 },
        },
      ],
      isLoading: false,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <PassRateTrendChart timeRange="24h" />
      </Wrapper>
    );

    // Both series should be rendered as Area components
    expect(screen.getByTestId("area-alpha")).toBeInTheDocument();
    expect(screen.getByTestId("area-zeta")).toBeInTheDocument();
  });

  it("handles single data point", () => {
    mockUseEvalPassRateTrends.mockReturnValue({
      data: [
        {
          timestamp: new Date("2026-01-01T10:00:00Z"),
          values: { tone: 0.9 },
        },
      ],
      isLoading: false,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <PassRateTrendChart timeRange="1h" />
      </Wrapper>
    );

    expect(screen.getByTestId("area-chart")).toBeInTheDocument();
    expect(screen.getByTestId("area-tone")).toBeInTheDocument();
  });
});

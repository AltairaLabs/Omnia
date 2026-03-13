/**
 * Tests for Quality page.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

// Mock hooks
const mockUseEvalSummary = vi.fn();
const mockUseEvalFilter = vi.fn();
const mockUseGrafana = vi.fn();
const mockBuildDashboardUrl = vi.fn();

vi.mock("@/hooks", () => ({
  useEvalSummary: (...args: unknown[]) => mockUseEvalSummary(...args),
  useEvalFilter: () => mockUseEvalFilter(),
  useGrafana: () => mockUseGrafana(),
  buildDashboardUrl: (...args: unknown[]) => mockBuildDashboardUrl(...args),
  GRAFANA_DASHBOARDS: { QUALITY: "omnia-quality" },
}));

vi.mock("@/components/layout", () => ({
  Header: ({ title, description }: { title: string; description: string }) => (
    <div data-testid="header">
      <h1>{title}</h1>
      <p>{description}</p>
    </div>
  ),
}));

vi.mock("@/components/quality/eval-score-breakdown", () => ({
  EvalScoreBreakdown: () => React.createElement("div", { "data-testid": "score-breakdown" }, "EvalScoreBreakdown"),
}));

vi.mock("@/components/quality/eval-score-trend-chart", () => ({
  EvalScoreTrendChart: () => React.createElement("div", { "data-testid": "trend-chart" }, "EvalScoreTrendChart"),
}));

import QualityPage from "./page";

const defaultEvalFilter = {
  agents: [],
  promptpacks: [],
  selectedAgent: undefined,
  selectedPromptPack: undefined,
  setAgent: vi.fn(),
  setPromptPack: vi.fn(),
  filter: {},
  isLoading: false,
};

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function TestQueryProvider({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  }
  return TestQueryProvider;
}

const mockSummaries = [
  {
    evalId: "tone",
    score: 0.85,
    metricType: "gauge" as const,
  },
  {
    evalId: "safety",
    score: 0.96,
    metricType: "gauge" as const,
  },
];

describe("QualityPage", () => {
  beforeEach(() => {
    mockUseEvalFilter.mockReturnValue(defaultEvalFilter);
    mockUseGrafana.mockReturnValue({ enabled: false, baseUrl: null, remotePath: "/grafana/", orgId: 1 });
    mockBuildDashboardUrl.mockReturnValue(null);
  });

  it("renders header with title", () => {
    mockUseEvalSummary.mockReturnValue({ data: [], isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    expect(screen.getByText("Quality")).toBeInTheDocument();
  });

  it("shows loading skeletons while data is fetching", () => {
    mockUseEvalSummary.mockReturnValue({ data: undefined, isLoading: true, error: null });

    const Wrapper = createWrapper();
    const { container } = render(<Wrapper><QualityPage /></Wrapper>);

    const skeletons = container.querySelectorAll('[class*="animate-pulse"], [data-slot="skeleton"]');
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("renders summary cards with score data", () => {
    mockUseEvalSummary.mockReturnValue({ data: mockSummaries, isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    expect(screen.getByText("Evals")).toBeInTheDocument();
    expect(screen.getByText("Avg Score")).toBeInTheDocument();
    expect(screen.getByText("Lowest Score")).toBeInTheDocument();
    // 2 evals
    expect(screen.getByText("2")).toBeInTheDocument();
    // Avg score: (85+96)/2 = 90.5 -> 91%
    expect(screen.getByText("91%")).toBeInTheDocument();
    // Lowest score: 85%
    expect(screen.getByText("85%")).toBeInTheDocument();
    // Lowest eval name
    expect(screen.getByText("tone")).toBeInTheDocument();
  });

  it("renders trend chart and breakdown components", () => {
    mockUseEvalSummary.mockReturnValue({ data: mockSummaries, isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    expect(screen.getByTestId("trend-chart")).toBeInTheDocument();
    expect(screen.getByTestId("score-breakdown")).toBeInTheDocument();
  });

  it("shows dash values when no eval data", () => {
    mockUseEvalSummary.mockReturnValue({ data: [], isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    const dashes = screen.getAllByText("-");
    expect(dashes).toHaveLength(2); // Avg Score and Lowest Score both show "-"
  });

  it("shows error alert when summary fetch fails", () => {
    mockUseEvalSummary.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("Failed to fetch"),
    });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    expect(screen.getByText("Error loading quality data")).toBeInTheDocument();
    expect(screen.getByText("Failed to fetch")).toBeInTheDocument();
  });

  it("renders time range selector", () => {
    mockUseEvalSummary.mockReturnValue({ data: [], isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    expect(screen.getByText("Last 24h")).toBeInTheDocument();
  });

  it("renders View in Grafana link when Grafana is enabled", () => {
    mockBuildDashboardUrl.mockReturnValue("https://grafana.local/grafana/d/omnia-quality/_?orgId=1");
    mockUseGrafana.mockReturnValue({ enabled: true, baseUrl: "https://grafana.local", remotePath: "/grafana/", orgId: 1 });
    mockUseEvalSummary.mockReturnValue({ data: [], isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    const link = screen.getByText("View in Grafana");
    expect(link).toBeInTheDocument();
    expect(link.closest("a")).toHaveAttribute("href", "https://grafana.local/grafana/d/omnia-quality/_?orgId=1");
    expect(link.closest("a")).toHaveAttribute("target", "_blank");
  });

  it("does not render View in Grafana link when Grafana is disabled", () => {
    mockBuildDashboardUrl.mockReturnValue(null);
    mockUseEvalSummary.mockReturnValue({ data: [], isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    expect(screen.queryByText("View in Grafana")).not.toBeInTheDocument();
  });

  it("renders agent filter when agents available", () => {
    mockUseEvalFilter.mockReturnValue({
      ...defaultEvalFilter,
      agents: ["chatbot", "support"],
    });
    mockUseEvalSummary.mockReturnValue({ data: [], isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    expect(screen.getByText("All agents")).toBeInTheDocument();
  });

  it("does not render tabs", () => {
    mockUseEvalSummary.mockReturnValue({ data: [], isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    expect(screen.queryByText("Overview")).not.toBeInTheDocument();
    expect(screen.queryByText("Assertions")).not.toBeInTheDocument();
  });
});

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
const mockUseRecentEvalFailures = vi.fn();
const mockUseEvalFilter = vi.fn();
const mockUseGrafana = vi.fn();
const mockBuildDashboardUrl = vi.fn();

vi.mock("@/hooks", () => ({
  useEvalSummary: (...args: unknown[]) => mockUseEvalSummary(...args),
  useRecentEvalFailures: (...args: unknown[]) => mockUseRecentEvalFailures(...args),
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

vi.mock("next/link", () => ({
  default: ({ children, href }: { children: React.ReactNode; href: string }) => (
    <a href={href}>{children}</a>
  ),
}));

vi.mock("date-fns", () => ({
  formatDistanceToNow: () => "2 hours ago",
}));

vi.mock("@/components/quality/assertion-type-breakdown", () => ({
  AssertionTypeBreakdown: () => React.createElement("div", { "data-testid": "assertion-breakdown" }, "AssertionTypeBreakdown"),
}));

vi.mock("@/components/quality/failing-sessions-table", () => ({
  FailingSessionsTable: () => React.createElement("div", { "data-testid": "failing-sessions" }, "FailingSessionsTable"),
}));

vi.mock("@/components/quality/pass-rate-trend-chart", () => ({
  PassRateTrendChart: () => React.createElement("div", { "data-testid": "trend-chart" }, "PassRateTrendChart"),
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
    evalType: "llm_judge",
    total: 100,
    passed: 85,
    failed: 15,
    passRate: 85.0,
    avgScore: 0.85,
    metricType: "gauge" as const,
  },
  {
    evalId: "safety",
    evalType: "rule",
    total: 50,
    passed: 48,
    failed: 2,
    passRate: 96.0,
    metricType: "gauge" as const,
  },
];

const mockFailures = {
  evalResults: [
    {
      id: "e1",
      sessionId: "s1",
      agentName: "agent-1",
      evalId: "tone",
      evalType: "llm_judge",
      passed: false,
      score: 0.3,
      createdAt: new Date().toISOString(),
    },
  ],
  total: 1,
};

describe("QualityPage", () => {
  beforeEach(() => {
    mockUseEvalFilter.mockReturnValue(defaultEvalFilter);
    mockUseGrafana.mockReturnValue({ enabled: false, baseUrl: null, remotePath: "/grafana/", orgId: 1 });
    mockBuildDashboardUrl.mockReturnValue(null);
  });

  it("renders header with title", () => {
    mockUseEvalSummary.mockReturnValue({ data: [], isLoading: false, error: null });
    mockUseRecentEvalFailures.mockReturnValue({ data: undefined, isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    expect(screen.getByText("Quality")).toBeInTheDocument();
  });

  it("shows loading skeletons while data is fetching", () => {
    mockUseEvalSummary.mockReturnValue({ data: undefined, isLoading: true, error: null });
    mockUseRecentEvalFailures.mockReturnValue({ data: undefined, isLoading: true, error: null });

    const Wrapper = createWrapper();
    const { container } = render(<Wrapper><QualityPage /></Wrapper>);

    // Skeleton elements should be present
    const skeletons = container.querySelectorAll('[class*="animate-pulse"], [data-slot="skeleton"]');
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("renders summary stats when data is loaded", () => {
    mockUseEvalSummary.mockReturnValue({ data: mockSummaries, isLoading: false, error: null });
    mockUseRecentEvalFailures.mockReturnValue({ data: mockFailures, isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    // Active Evals: 2 metrics
    expect(screen.getByText("Active Evals")).toBeInTheDocument();
    // Overall pass rate: mean of 85.0 and 96.0 = 90.5%
    expect(screen.getByText("90.5%")).toBeInTheDocument();
    // Passing: 1 (safety at 96% >= 90), Failing: 0 (none < 70)
    expect(screen.getByText("Passing")).toBeInTheDocument();
    expect(screen.getByText("Failing")).toBeInTheDocument();
  });

  it("renders eval metrics table", () => {
    mockUseEvalSummary.mockReturnValue({ data: mockSummaries, isLoading: false, error: null });
    mockUseRecentEvalFailures.mockReturnValue({ data: mockFailures, isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    expect(screen.getByText("Eval Metrics")).toBeInTheDocument();
    expect(screen.getByText("safety")).toBeInTheDocument();
    expect(screen.getByText("85.0%")).toBeInTheDocument();
    expect(screen.getByText("96.0%")).toBeInTheDocument();
  });

  it("renders recent failures", () => {
    mockUseEvalSummary.mockReturnValue({ data: mockSummaries, isLoading: false, error: null });
    mockUseRecentEvalFailures.mockReturnValue({ data: mockFailures, isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    expect(screen.getByText("Recent Failures")).toBeInTheDocument();
    // "tone" appears in both eval table and failures — use getAllByText
    const toneElements = screen.getAllByText("tone");
    expect(toneElements.length).toBeGreaterThanOrEqual(2);
    expect(screen.getByText("agent-1")).toBeInTheDocument();
    expect(screen.getByText("0.30")).toBeInTheDocument();
  });

  it("shows empty state when no eval data", () => {
    mockUseEvalSummary.mockReturnValue({ data: [], isLoading: false, error: null });
    mockUseRecentEvalFailures.mockReturnValue({
      data: { evalResults: [], total: 0 },
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    expect(screen.getByText("No eval data available")).toBeInTheDocument();
    expect(screen.getByText("No recent failures")).toBeInTheDocument();
  });

  it("excludes counter metrics from overall pass rate", () => {
    const mixedSummaries = [
      ...mockSummaries,
      {
        evalId: "executed_total",
        evalType: "counter",
        total: 47,
        passed: 0,
        failed: 0,
        passRate: 0,
        metricType: "counter" as const,
      },
    ];
    mockUseEvalSummary.mockReturnValue({ data: mixedSummaries, isLoading: false, error: null });
    mockUseRecentEvalFailures.mockReturnValue({ data: mockFailures, isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    // Overall pass rate should only average gauge metrics: (85.0 + 96.0) / 2 = 90.5%
    // NOT (85.0 + 96.0 + 0) / 3 = 60.3%
    expect(screen.getByText("90.5%")).toBeInTheDocument();
    // Active Evals count includes all 3
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("renders counter metrics with raw count instead of pass rate", () => {
    const counterSummaries = [
      {
        evalId: "executed_total",
        evalType: "counter",
        total: 47,
        passed: 0,
        failed: 0,
        passRate: 0,
        metricType: "counter" as const,
      },
    ];
    mockUseEvalSummary.mockReturnValue({ data: counterSummaries, isLoading: false, error: null });
    mockUseRecentEvalFailures.mockReturnValue({ data: { evalResults: [], total: 0 }, isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    expect(screen.getByText("47")).toBeInTheDocument();
    expect(screen.getByText("count")).toBeInTheDocument();
  });

  it("shows error alert when summary fetch fails", () => {
    mockUseEvalSummary.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("Failed to fetch"),
    });
    mockUseRecentEvalFailures.mockReturnValue({ data: undefined, isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    expect(screen.getByText("Error loading quality data")).toBeInTheDocument();
    expect(screen.getByText("Failed to fetch")).toBeInTheDocument();
  });

  it("renders time range selector", () => {
    mockUseEvalSummary.mockReturnValue({ data: [], isLoading: false, error: null });
    mockUseRecentEvalFailures.mockReturnValue({ data: undefined, isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    // Default time range is 24h
    expect(screen.getByText("Last 24h")).toBeInTheDocument();
  });

  it("renders View in Grafana link when Grafana is enabled", () => {
    mockBuildDashboardUrl.mockReturnValue("https://grafana.local/grafana/d/omnia-quality/_?orgId=1");
    mockUseGrafana.mockReturnValue({ enabled: true, baseUrl: "https://grafana.local", remotePath: "/grafana/", orgId: 1 });
    mockUseEvalSummary.mockReturnValue({ data: [], isLoading: false, error: null });
    mockUseRecentEvalFailures.mockReturnValue({ data: undefined, isLoading: false, error: null });

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
    mockUseRecentEvalFailures.mockReturnValue({ data: undefined, isLoading: false, error: null });

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
    mockUseRecentEvalFailures.mockReturnValue({ data: undefined, isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    expect(screen.getByText("All agents")).toBeInTheDocument();
  });
});

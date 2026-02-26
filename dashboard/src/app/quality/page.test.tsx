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
const mockUseAgents = vi.fn();
const mockUseEvalMetrics = vi.fn();

vi.mock("@/hooks", () => ({
  useEvalSummary: (...args: unknown[]) => mockUseEvalSummary(...args),
  useRecentEvalFailures: (...args: unknown[]) => mockUseRecentEvalFailures(...args),
  useAgents: () => mockUseAgents(),
  useEvalMetrics: () => mockUseEvalMetrics(),
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

vi.mock("@/components/quality/alert-config-panel", () => ({
  AlertConfigPanel: () => React.createElement("div", { "data-testid": "alert-config" }, "AlertConfigPanel"),
  buildAlertThresholdMap: () => new Map(),
  loadAlerts: () => [],
}));

import QualityPage from "./page";

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
  },
  {
    evalId: "safety",
    evalType: "rule",
    total: 50,
    passed: 48,
    failed: 2,
    passRate: 96.0,
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
    mockUseAgents.mockReturnValue({
      data: [
        { metadata: { name: "agent-1" } },
        { metadata: { name: "agent-2" } },
      ],
    });
    mockUseEvalMetrics.mockReturnValue({ data: [], isLoading: false, error: null });
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

    // Total evals: 100 + 50 = 150
    expect(screen.getByText("150")).toBeInTheDocument();
    // Overall pass rate: (85 + 48) / 150 * 100 = 88.7%
    expect(screen.getByText("88.7%")).toBeInTheDocument();
    // Total passed: 85 + 48 = 133
    expect(screen.getByText("133")).toBeInTheDocument();
    // Eval types: verify the label exists
    expect(screen.getByText("Eval Types")).toBeInTheDocument();
  });

  it("renders eval pass rate table", () => {
    mockUseEvalSummary.mockReturnValue({ data: mockSummaries, isLoading: false, error: null });
    mockUseRecentEvalFailures.mockReturnValue({ data: mockFailures, isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    expect(screen.getByText("Pass Rate by Eval")).toBeInTheDocument();
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

  it("renders time range selector with 7d default", () => {
    mockUseEvalSummary.mockReturnValue({ data: [], isLoading: false, error: null });
    mockUseRecentEvalFailures.mockReturnValue({ data: undefined, isLoading: false, error: null });

    const Wrapper = createWrapper();
    render(<Wrapper><QualityPage /></Wrapper>);

    // The select should show "Last 7d" as the default
    expect(screen.getByText("Last 7d")).toBeInTheDocument();
  });
});

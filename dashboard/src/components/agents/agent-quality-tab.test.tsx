/**
 * Tests for AgentQualityTab component.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AgentQualityTab } from "./agent-quality-tab";

// Mock eval hooks
const mockUseEvalSummary = vi.fn();
vi.mock("@/hooks", () => ({
  useEvalSummary: (...args: unknown[]) => mockUseEvalSummary(...args),
}));

// Mock quality components to isolate unit tests
vi.mock("@/components/quality/eval-score-breakdown", () => ({
  EvalScoreBreakdown: ({ filter }: { filter?: { agent?: string } }) => (
    <div data-testid="score-breakdown" data-agent={filter?.agent} />
  ),
}));

vi.mock("@/components/quality/eval-score-trend-chart", () => ({
  EvalScoreTrendChart: ({ filter, timeRange }: { filter?: { agent?: string }; timeRange?: string }) => (
    <div data-testid="trend-chart" data-agent={filter?.agent} data-range={timeRange} />
  ),
}));

function renderTab(agentName = "my-agent") {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentQualityTab agentName={agentName} />
    </QueryClientProvider>
  );
}

describe("AgentQualityTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseEvalSummary.mockReturnValue({ data: [], isLoading: false });
  });

  it("renders summary cards with score labels", () => {
    renderTab();
    expect(screen.getByText("Evals")).toBeInTheDocument();
    expect(screen.getByText("Avg Score")).toBeInTheDocument();
    expect(screen.getByText("Lowest Score")).toBeInTheDocument();
  });

  it("passes agent filter to child components", () => {
    renderTab("test-agent");
    expect(screen.getByTestId("score-breakdown")).toHaveAttribute("data-agent", "test-agent");
    expect(screen.getByTestId("trend-chart")).toHaveAttribute("data-agent", "test-agent");
  });

  it("passes agent filter to useEvalSummary", () => {
    renderTab("test-agent");
    expect(mockUseEvalSummary).toHaveBeenCalledWith({ agent: "test-agent" });
  });

  it("computes score stats from summaries", () => {
    mockUseEvalSummary.mockReturnValue({
      data: [
        { evalId: "a", score: 0.95, metricType: "gauge" },
        { evalId: "b", score: 0.60, metricType: "gauge" },
        { evalId: "c", score: 0.80, metricType: "gauge" },
        { evalId: "d", score: 100, metricType: "counter" },
      ],
      isLoading: false,
    });
    renderTab();
    // 4 total evals
    expect(screen.getByText("4")).toBeInTheDocument();
    // avg score of gauges: (95+60+80)/3 = 78.3 -> 78%
    expect(screen.getByText("78%")).toBeInTheDocument();
    // lowest score: 60%
    expect(screen.getByText("60%")).toBeInTheDocument();
    // lowest eval name
    expect(screen.getByText("b")).toBeInTheDocument();
  });

  it("shows loading skeletons", () => {
    mockUseEvalSummary.mockReturnValue({ data: undefined, isLoading: true });
    const { container } = renderTab();
    const skeletons = container.querySelectorAll("[class*='skeleton' i], [data-slot='skeleton']");
    expect(skeletons.length).toBeGreaterThanOrEqual(3);
  });

  it("changes time range on button click", () => {
    renderTab();
    expect(screen.getByTestId("trend-chart")).toHaveAttribute("data-range", "24h");
    fireEvent.click(screen.getByText("7d"));
    expect(screen.getByTestId("trend-chart")).toHaveAttribute("data-range", "7d");
  });

  it("handles empty summaries gracefully", () => {
    mockUseEvalSummary.mockReturnValue({ data: [], isLoading: false });
    renderTab();
    const dashes = screen.getAllByText("-");
    expect(dashes).toHaveLength(2); // Avg Score and Lowest Score
  });
});

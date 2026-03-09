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
vi.mock("@/components/quality/assertion-type-breakdown", () => ({
  AssertionTypeBreakdown: ({ filter }: { filter?: { agent?: string } }) => (
    <div data-testid="assertion-breakdown" data-agent={filter?.agent} />
  ),
}));

vi.mock("@/components/quality/pass-rate-trend-chart", () => ({
  PassRateTrendChart: ({ filter, timeRange }: { filter?: { agent?: string }; timeRange?: string }) => (
    <div data-testid="trend-chart" data-agent={filter?.agent} data-range={timeRange} />
  ),
}));

vi.mock("@/components/quality/failing-sessions-table", () => ({
  FailingSessionsTable: ({ agentName }: { agentName?: string }) => (
    <div data-testid="failing-sessions" data-agent={agentName} />
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

  it("renders summary cards", () => {
    renderTab();
    expect(screen.getByText("Active Evals")).toBeInTheDocument();
    expect(screen.getByText("Avg Pass Rate")).toBeInTheDocument();
    expect(screen.getByText("Passing")).toBeInTheDocument();
    expect(screen.getByText("Failing")).toBeInTheDocument();
  });

  it("passes agent filter to child components", () => {
    renderTab("test-agent");
    expect(screen.getByTestId("assertion-breakdown")).toHaveAttribute("data-agent", "test-agent");
    expect(screen.getByTestId("trend-chart")).toHaveAttribute("data-agent", "test-agent");
    expect(screen.getByTestId("failing-sessions")).toHaveAttribute("data-agent", "test-agent");
  });

  it("passes agent filter to useEvalSummary", () => {
    renderTab("test-agent");
    expect(mockUseEvalSummary).toHaveBeenCalledWith({ agent: "test-agent" });
  });

  it("computes stats from summaries", () => {
    mockUseEvalSummary.mockReturnValue({
      data: [
        { evalId: "a", passRate: 95, metricType: "gauge" },
        { evalId: "b", passRate: 60, metricType: "gauge" },
        { evalId: "c", passRate: 80, metricType: "gauge" },
        { evalId: "d", passRate: 100, metricType: "counter" },
      ],
      isLoading: false,
    });
    renderTab();
    // 4 total evals
    expect(screen.getByText("4")).toBeInTheDocument();
    // avg pass rate of gauges: (95+60+80)/3 = 78.3%
    expect(screen.getByText("78.3%")).toBeInTheDocument();
    // 1 passing (>=90) and 1 failing (<70) — both render "1"
    const ones = screen.getAllByText("1");
    expect(ones).toHaveLength(2);
  });

  it("shows loading skeletons", () => {
    mockUseEvalSummary.mockReturnValue({ data: undefined, isLoading: true });
    const { container } = renderTab();
    // Should have skeleton elements for the 4 summary cards
    const skeletons = container.querySelectorAll("[class*='skeleton' i], [data-slot='skeleton']");
    expect(skeletons.length).toBeGreaterThanOrEqual(4);
  });

  it("changes time range on button click", () => {
    renderTab();
    // Default is 24h
    expect(screen.getByTestId("trend-chart")).toHaveAttribute("data-range", "24h");
    // Click 7d
    fireEvent.click(screen.getByText("7d"));
    expect(screen.getByTestId("trend-chart")).toHaveAttribute("data-range", "7d");
  });

  it("handles empty summaries gracefully", () => {
    mockUseEvalSummary.mockReturnValue({ data: [], isLoading: false });
    renderTab();
    expect(screen.getByText("0.0%")).toBeInTheDocument();
  });
});

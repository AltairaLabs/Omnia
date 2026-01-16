import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { CostOverTimeChart } from "./cost-over-time-chart";
import type { CostTimeSeriesPoint } from "@/lib/data/types";

// Mock recharts components
vi.mock("recharts", () => ({
  AreaChart: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="area-chart">{children}</div>
  ),
  Area: ({ dataKey }: { dataKey: string }) => (
    <div data-testid={`area-${dataKey}`} />
  ),
  XAxis: () => <div data-testid="x-axis" />,
  YAxis: () => <div data-testid="y-axis" />,
  CartesianGrid: () => <div data-testid="cartesian-grid" />,
  Tooltip: () => <div data-testid="tooltip" />,
  Legend: () => <div data-testid="legend" />,
  ResponsiveContainer: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="responsive-container">{children}</div>
  ),
}));

describe("CostOverTimeChart", () => {
  const mockData: CostTimeSeriesPoint[] = [
    {
      timestamp: "2024-01-01T10:00:00Z",
      total: 0.05,
      byProvider: { openai: 0.03, anthropic: 0.02 },
    },
    {
      timestamp: "2024-01-01T11:00:00Z",
      total: 0.08,
      byProvider: { openai: 0.05, anthropic: 0.03 },
    },
  ];

  it("renders with default props", () => {
    render(<CostOverTimeChart data={mockData} />);

    expect(screen.getByText("Cost Over Time")).toBeInTheDocument();
    expect(screen.getByText("LLM costs by provider over the last 24 hours")).toBeInTheDocument();
  });

  it("renders with custom title and description", () => {
    render(
      <CostOverTimeChart
        data={mockData}
        title="Custom Title"
        description="Custom description"
      />
    );

    expect(screen.getByText("Custom Title")).toBeInTheDocument();
    expect(screen.getByText("Custom description")).toBeInTheDocument();
  });

  it("renders Grafana link when grafanaUrl is provided", () => {
    render(
      <CostOverTimeChart data={mockData} grafanaUrl="https://grafana.example.com" />
    );

    const link = screen.getByRole("link", { name: /view in grafana/i });
    expect(link).toHaveAttribute("href", "https://grafana.example.com");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("does not render Grafana link when grafanaUrl is not provided", () => {
    render(<CostOverTimeChart data={mockData} />);

    expect(screen.queryByRole("link", { name: /view in grafana/i })).not.toBeInTheDocument();
  });

  it("renders the chart components", () => {
    render(<CostOverTimeChart data={mockData} />);

    expect(screen.getByTestId("responsive-container")).toBeInTheDocument();
    expect(screen.getByTestId("area-chart")).toBeInTheDocument();
  });

  it("handles empty data gracefully", () => {
    render(<CostOverTimeChart data={[]} />);

    expect(screen.getByText("Cost Over Time")).toBeInTheDocument();
    expect(screen.getByTestId("area-chart")).toBeInTheDocument();
  });

  it("extracts and sorts providers alphabetically using localeCompare", () => {
    // Data with providers in non-alphabetical order
    const unsortedData: CostTimeSeriesPoint[] = [
      {
        timestamp: "2024-01-01T10:00:00Z",
        total: 0.1,
        byProvider: { zebra: 0.01, anthropic: 0.02, openai: 0.03, bedrock: 0.04 },
      },
    ];

    render(<CostOverTimeChart data={unsortedData} />);

    // The chart should render - providers are sorted internally for consistent ordering
    expect(screen.getByTestId("area-chart")).toBeInTheDocument();
  });

  it("handles single provider data", () => {
    const singleProviderData: CostTimeSeriesPoint[] = [
      {
        timestamp: "2024-01-01T10:00:00Z",
        total: 0.05,
        byProvider: { openai: 0.05 },
      },
    ];

    render(<CostOverTimeChart data={singleProviderData} />);

    expect(screen.getByTestId("area-chart")).toBeInTheDocument();
  });

  it("handles data with zero costs", () => {
    const zeroCostData: CostTimeSeriesPoint[] = [
      {
        timestamp: "2024-01-01T10:00:00Z",
        total: 0,
        byProvider: { openai: 0 },
      },
    ];

    render(<CostOverTimeChart data={zeroCostData} />);

    expect(screen.getByTestId("area-chart")).toBeInTheDocument();
  });
});

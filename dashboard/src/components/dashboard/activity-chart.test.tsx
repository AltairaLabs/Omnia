/**
 * Tests for ActivityChart component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { ActivityChart } from "./activity-chart";
import { useAgentActivity, type ActivityDataPoint } from "@/hooks";

vi.mock("@/hooks", () => ({
  useAgentActivity: vi.fn(),
}));

// Mock recharts to avoid rendering issues in tests
vi.mock("recharts", () => ({
  AreaChart: ({ children }: { children: React.ReactNode }) => <div data-testid="area-chart">{children}</div>,
  Area: () => <div data-testid="area" />,
  XAxis: () => <div data-testid="x-axis" />,
  YAxis: () => <div data-testid="y-axis" />,
  CartesianGrid: () => <div data-testid="cartesian-grid" />,
  Tooltip: () => <div data-testid="tooltip" />,
  ResponsiveContainer: ({ children }: { children: React.ReactNode }) => <div data-testid="responsive-container">{children}</div>,
}));

const mockUseAgentActivity = vi.mocked(useAgentActivity);

const mockActivityData: ActivityDataPoint[] = [
  { time: "00:00", requests: 100, sessions: 10 },
  { time: "01:00", requests: 150, sessions: 15 },
  { time: "02:00", requests: 200, sessions: 20 },
  { time: "03:00", requests: 175, sessions: 18 },
];

describe("ActivityChart", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render loading state", () => {
    mockUseAgentActivity.mockReturnValue({
      data: [],
      available: false,
      isDemo: false,
      isLoading: true,
      error: null,
    });

    render(<ActivityChart />);

    expect(screen.getByText("Agent Activity")).toBeInTheDocument();
    expect(screen.getByText("Requests per hour across all agents")).toBeInTheDocument();
  });

  it("should render unavailable state when Prometheus not configured", () => {
    mockUseAgentActivity.mockReturnValue({
      data: [],
      available: false,
      isDemo: false,
      isLoading: false,
      error: null,
    });

    render(<ActivityChart />);

    expect(screen.getByText("No activity data available")).toBeInTheDocument();
    expect(screen.getByText("Prometheus metrics not configured")).toBeInTheDocument();
  });

  it("should render empty state when no activity in last 24 hours", () => {
    mockUseAgentActivity.mockReturnValue({
      data: [],
      available: true,
      isDemo: false,
      isLoading: false,
      error: null,
    });

    render(<ActivityChart />);

    expect(screen.getByText("No activity in the last 24 hours")).toBeInTheDocument();
  });

  it("should render chart with data", () => {
    mockUseAgentActivity.mockReturnValue({
      data: mockActivityData,
      available: true,
      isDemo: false,
      isLoading: false,
      error: null,
    });

    render(<ActivityChart />);

    expect(screen.getByTestId("responsive-container")).toBeInTheDocument();
    expect(screen.getByTestId("area-chart")).toBeInTheDocument();
  });

  it("should show demo badge when in demo mode", () => {
    mockUseAgentActivity.mockReturnValue({
      data: mockActivityData,
      available: true,
      isDemo: true,
      isLoading: false,
      error: null,
    });

    render(<ActivityChart />);

    expect(screen.getByText("Demo Data")).toBeInTheDocument();
  });

  it("should not show demo badge when not in demo mode", () => {
    mockUseAgentActivity.mockReturnValue({
      data: mockActivityData,
      available: true,
      isDemo: false,
      isLoading: false,
      error: null,
    });

    render(<ActivityChart />);

    expect(screen.queryByText("Demo Data")).not.toBeInTheDocument();
  });

  it("should render empty state in demo mode when no data", () => {
    mockUseAgentActivity.mockReturnValue({
      data: [],
      available: false,
      isDemo: true,
      isLoading: false,
      error: null,
    });

    render(<ActivityChart />);

    // In demo mode with no data, should still show empty state
    expect(screen.getByText("No activity in the last 24 hours")).toBeInTheDocument();
  });
});

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { EventsPanel } from "./events-panel";

// Mock the hooks
const mockRefetch = vi.fn();
const mockUseAgentEvents = vi.fn();

vi.mock("@/hooks", () => ({
  useAgentEvents: (...args: unknown[]) => mockUseAgentEvents(...args),
}));

// Mock data
const mockEvents = [
  {
    type: "Normal" as const,
    reason: "Pulled",
    message: "Successfully pulled image \"nginx:latest\"",
    firstTimestamp: "2024-01-01T10:00:00Z",
    lastTimestamp: "2024-01-01T10:00:00Z",
    count: 1,
    source: { component: "kubelet", host: "node-1" },
    involvedObject: { kind: "Pod", name: "my-agent-abc123", namespace: "test-ns" },
  },
  {
    type: "Warning" as const,
    reason: "BackOff",
    message: "Back-off restarting failed container",
    firstTimestamp: "2024-01-01T10:05:00Z",
    lastTimestamp: "2024-01-01T10:10:00Z",
    count: 5,
    source: { component: "kubelet", host: "node-1" },
    involvedObject: { kind: "Pod", name: "my-agent-def456", namespace: "test-ns" },
  },
  {
    type: "Normal" as const,
    reason: "Created",
    message: "Created container runtime",
    firstTimestamp: "2024-01-01T10:00:00Z",
    lastTimestamp: "2024-01-01T10:00:00Z",
    count: 1,
    source: { component: "kubelet", host: "node-1" },
    involvedObject: { kind: "Pod", name: "my-agent-abc123", namespace: "test-ns" },
  },
];

describe("EventsPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockRefetch.mockClear();
  });

  it("renders loading state", () => {
    mockUseAgentEvents.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    expect(screen.getByTestId("events-loading")).toBeInTheDocument();
    expect(screen.getByText("Recent Events")).toBeInTheDocument();
  });

  it("renders error state", () => {
    mockUseAgentEvents.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("Failed to fetch events"),
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    expect(screen.getByTestId("events-error")).toBeInTheDocument();
    expect(screen.getByText("Failed to load events")).toBeInTheDocument();
  });

  it("renders empty state when no events", () => {
    mockUseAgentEvents.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    expect(screen.getByTestId("events-empty")).toBeInTheDocument();
    expect(screen.getByText("No recent events")).toBeInTheDocument();
  });

  it("renders empty state when events is undefined", () => {
    mockUseAgentEvents.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    expect(screen.getByTestId("events-empty")).toBeInTheDocument();
  });

  it("renders events table with data", () => {
    mockUseAgentEvents.mockReturnValue({
      data: mockEvents,
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    expect(screen.getByTestId("events-table")).toBeInTheDocument();
    expect(screen.getByText("Type")).toBeInTheDocument();
    expect(screen.getByText("Reason")).toBeInTheDocument();
    expect(screen.getByText("Object")).toBeInTheDocument();
    expect(screen.getByText("Message")).toBeInTheDocument();
    expect(screen.getByText("Age")).toBeInTheDocument();
  });

  it("renders event rows with correct data", () => {
    mockUseAgentEvents.mockReturnValue({
      data: mockEvents,
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    // Check for event reasons
    expect(screen.getByText("Pulled")).toBeInTheDocument();
    expect(screen.getByText("BackOff")).toBeInTheDocument();
    expect(screen.getByText("Created")).toBeInTheDocument();

    // Check for event messages
    expect(screen.getByText('Successfully pulled image "nginx:latest"')).toBeInTheDocument();
    expect(screen.getByText("Back-off restarting failed container")).toBeInTheDocument();
  });

  it("shows event count when greater than 1", () => {
    mockUseAgentEvents.mockReturnValue({
      data: mockEvents,
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    // The BackOff event has count: 5
    expect(screen.getByText("×5")).toBeInTheDocument();
  });

  it("renders refresh button", () => {
    mockUseAgentEvents.mockReturnValue({
      data: mockEvents,
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    expect(screen.getByText("Refresh")).toBeInTheDocument();
  });

  it("calls refetch when refresh button is clicked", async () => {
    mockUseAgentEvents.mockReturnValue({
      data: mockEvents,
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    const user = userEvent.setup();
    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    await user.click(screen.getByText("Refresh"));

    expect(mockRefetch).toHaveBeenCalled();
  });

  it("disables refresh button while fetching", () => {
    mockUseAgentEvents.mockReturnValue({
      data: mockEvents,
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: true,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    const refreshButton = screen.getByText("Refresh").closest("button");
    expect(refreshButton).toBeDisabled();
  });

  it("passes correct parameters to useAgentEvents", () => {
    mockUseAgentEvents.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="my-agent" workspace="my-workspace" />);

    expect(mockUseAgentEvents).toHaveBeenCalledWith("my-agent", "my-workspace");
  });

  it("truncates long object names", () => {
    const longNameEvent = {
      type: "Normal" as const,
      reason: "Started",
      message: "Started container",
      firstTimestamp: "2024-01-01T10:00:00Z",
      lastTimestamp: "2024-01-01T10:00:00Z",
      count: 1,
      source: { component: "kubelet" },
      involvedObject: {
        kind: "Pod",
        name: "my-very-long-agent-name-with-hash-abc123def456",
        namespace: "test-ns",
      },
    };

    mockUseAgentEvents.mockReturnValue({
      data: [longNameEvent],
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    // The name should be truncated with ellipsis
    const objectCell = screen.getByText(/Pod\/my-very-long-agent-name/);
    expect(objectCell).toBeInTheDocument();
    // Full name should be in title attribute
    expect(objectCell.getAttribute("title")).toBe(
      "Pod/my-very-long-agent-name-with-hash-abc123def456"
    );
  });

  it("renders card header with title and description", () => {
    mockUseAgentEvents.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    expect(screen.getByText("Recent Events")).toBeInTheDocument();
    expect(screen.getByText("Kubernetes events related to this agent")).toBeInTheDocument();
  });

  it("renders events panel container with correct testid", () => {
    mockUseAgentEvents.mockReturnValue({
      data: mockEvents,
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    expect(screen.getByTestId("events-panel")).toBeInTheDocument();
  });

  it("renders event rows with correct testid", () => {
    mockUseAgentEvents.mockReturnValue({
      data: mockEvents,
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    const eventRows = screen.getAllByTestId("event-row");
    expect(eventRows).toHaveLength(3);
  });
});

describe("formatRelativeTime", () => {
  // These tests verify the time formatting indirectly through the component
  it("shows 'just now' for very recent events", () => {
    const recentEvent = {
      type: "Normal" as const,
      reason: "Created",
      message: "Created container",
      firstTimestamp: new Date().toISOString(),
      lastTimestamp: new Date().toISOString(),
      count: 1,
      source: { component: "kubelet" },
      involvedObject: { kind: "Pod", name: "test-pod", namespace: "test-ns" },
    };

    mockUseAgentEvents.mockReturnValue({
      data: [recentEvent],
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    expect(screen.getByText("just now")).toBeInTheDocument();
  });

  it("shows minutes ago for events within an hour", () => {
    const tenMinutesAgo = new Date(Date.now() - 10 * 60 * 1000).toISOString();
    const event = {
      type: "Normal" as const,
      reason: "Created",
      message: "Created container",
      firstTimestamp: tenMinutesAgo,
      lastTimestamp: tenMinutesAgo,
      count: 1,
      source: { component: "kubelet" },
      involvedObject: { kind: "Pod", name: "test-pod", namespace: "test-ns" },
    };

    mockUseAgentEvents.mockReturnValue({
      data: [event],
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    expect(screen.getByText("10m ago")).toBeInTheDocument();
  });

  it("shows hours ago for events within a day", () => {
    const threeHoursAgo = new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString();
    const event = {
      type: "Normal" as const,
      reason: "Created",
      message: "Created container",
      firstTimestamp: threeHoursAgo,
      lastTimestamp: threeHoursAgo,
      count: 1,
      source: { component: "kubelet" },
      involvedObject: { kind: "Pod", name: "test-pod", namespace: "test-ns" },
    };

    mockUseAgentEvents.mockReturnValue({
      data: [event],
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    expect(screen.getByText("3h ago")).toBeInTheDocument();
  });

  it("shows days ago for older events", () => {
    const twoDaysAgo = new Date(Date.now() - 2 * 24 * 60 * 60 * 1000).toISOString();
    const event = {
      type: "Normal" as const,
      reason: "Created",
      message: "Created container",
      firstTimestamp: twoDaysAgo,
      lastTimestamp: twoDaysAgo,
      count: 1,
      source: { component: "kubelet" },
      involvedObject: { kind: "Pod", name: "test-pod", namespace: "test-ns" },
    };

    mockUseAgentEvents.mockReturnValue({
      data: [event],
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    expect(screen.getByText("2d ago")).toBeInTheDocument();
  });
});

describe("truncateObjectName", () => {
  it("does not truncate short names", () => {
    const shortNameEvent = {
      type: "Normal" as const,
      reason: "Created",
      message: "Created container",
      firstTimestamp: "2024-01-01T10:00:00Z",
      lastTimestamp: "2024-01-01T10:00:00Z",
      count: 1,
      source: { component: "kubelet" },
      involvedObject: { kind: "Pod", name: "short-name", namespace: "test-ns" },
    };

    mockUseAgentEvents.mockReturnValue({
      data: [shortNameEvent],
      isLoading: false,
      error: null,
      refetch: mockRefetch,
      isFetching: false,
    });

    render(<EventsPanel agentName="test-agent" workspace="test-workspace" />);

    expect(screen.getByText("Pod/short-name")).toBeInTheDocument();
  });
});

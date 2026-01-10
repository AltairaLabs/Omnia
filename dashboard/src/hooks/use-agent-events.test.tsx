import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useAgentEvents } from "./use-agent-events";

// Mock event data
const mockEvents = [
  {
    type: "Normal",
    reason: "AgentStarted",
    message: "Agent started successfully",
    lastTimestamp: "2024-01-15T10:00:00Z",
    count: 1,
  },
  {
    type: "Warning",
    reason: "HighMemory",
    message: "Memory usage above 80%",
    lastTimestamp: "2024-01-15T10:05:00Z",
    count: 3,
  },
];

// Mock useDataService
const mockGetAgentEvents = vi.fn().mockResolvedValue(mockEvents);
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    getAgentEvents: mockGetAgentEvents,
  }),
}));

function TestWrapper({ children }: { children: React.ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

describe("useAgentEvents", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should fetch events for an agent", async () => {
    const { result } = renderHook(() => useAgentEvents("my-agent", "production"), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockEvents);
  });

  it("should be in loading state initially", () => {
    const { result } = renderHook(() => useAgentEvents("my-agent", "production"), {
      wrapper: TestWrapper,
    });

    expect(result.current.isLoading).toBe(true);
  });

  it("should call getAgentEvents with correct parameters", async () => {
    renderHook(() => useAgentEvents("test-agent", "staging"), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(mockGetAgentEvents).toHaveBeenCalled();
    });

    expect(mockGetAgentEvents).toHaveBeenCalledWith("staging", "test-agent");
  });

  it("should not fetch when agentName is empty", () => {
    renderHook(() => useAgentEvents("", "production"), {
      wrapper: TestWrapper,
    });

    expect(mockGetAgentEvents).not.toHaveBeenCalled();
  });

  it("should not fetch when namespace is empty", () => {
    renderHook(() => useAgentEvents("my-agent", ""), {
      wrapper: TestWrapper,
    });

    expect(mockGetAgentEvents).not.toHaveBeenCalled();
  });

  it("should handle empty events response", async () => {
    mockGetAgentEvents.mockResolvedValueOnce([]);

    const { result } = renderHook(() => useAgentEvents("my-agent", "production"), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual([]);
  });
});

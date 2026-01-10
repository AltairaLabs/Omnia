import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useLogs } from "./use-logs";

// Mock log data
const mockLogs = [
  { timestamp: "2024-01-15T10:00:00Z", level: "INFO", message: "Agent started" },
  { timestamp: "2024-01-15T10:00:01Z", level: "INFO", message: "Processing request" },
  { timestamp: "2024-01-15T10:00:02Z", level: "DEBUG", message: "Request completed" },
];

// Mock useDataService
const mockGetAgentLogs = vi.fn().mockResolvedValue(mockLogs);
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    isDemo: false,
    getAgentLogs: mockGetAgentLogs,
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

describe("useLogs", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should fetch logs for an agent", async () => {
    const { result } = renderHook(() => useLogs("production", "my-agent"), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockLogs);
  });

  it("should use default options when not provided", async () => {
    renderHook(() => useLogs("production", "my-agent"), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(mockGetAgentLogs).toHaveBeenCalled();
    });

    expect(mockGetAgentLogs).toHaveBeenCalledWith("production", "my-agent", {
      tailLines: 200,
      sinceSeconds: 3600,
      container: undefined,
    });
  });

  it("should accept custom options", async () => {
    const options = {
      tailLines: 100,
      sinceSeconds: 1800,
      container: "main",
    };

    renderHook(() => useLogs("staging", "test-agent", options), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(mockGetAgentLogs).toHaveBeenCalled();
    });

    expect(mockGetAgentLogs).toHaveBeenCalledWith("staging", "test-agent", {
      tailLines: 100,
      sinceSeconds: 1800,
      container: "main",
    });
  });

  it("should be in loading state initially", () => {
    const { result } = renderHook(() => useLogs("production", "my-agent"), {
      wrapper: TestWrapper,
    });

    expect(result.current.isLoading).toBe(true);
  });

  it("should handle namespace and name parameters", async () => {
    const { result } = renderHook(() => useLogs("custom-ns", "custom-agent"), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(mockGetAgentLogs).toHaveBeenCalledWith(
      "custom-ns",
      "custom-agent",
      expect.any(Object)
    );
  });
});

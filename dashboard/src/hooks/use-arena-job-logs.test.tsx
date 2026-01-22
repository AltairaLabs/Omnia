import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useArenaJobLogs } from "./use-arena-job-logs";

// Mock log data
const mockLogs = [
  { timestamp: "2024-01-15T10:00:00Z", level: "INFO", message: "Arena job started" },
  { timestamp: "2024-01-15T10:00:01Z", level: "INFO", message: "Running evaluation" },
  { timestamp: "2024-01-15T10:00:02Z", level: "DEBUG", message: "Task completed" },
];

// Mock useDataService
const mockGetArenaJobLogs = vi.fn().mockResolvedValue(mockLogs);
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    isDemo: false,
    getArenaJobLogs: mockGetArenaJobLogs,
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

describe("useArenaJobLogs", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should fetch logs for an arena job", async () => {
    const { result } = renderHook(() => useArenaJobLogs("production", "eval-job-1"), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockLogs);
  });

  it("should use default options when not provided", async () => {
    renderHook(() => useArenaJobLogs("production", "eval-job-1"), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(mockGetArenaJobLogs).toHaveBeenCalled();
    });

    expect(mockGetArenaJobLogs).toHaveBeenCalledWith("production", "eval-job-1", {
      tailLines: 200,
      sinceSeconds: 3600,
    });
  });

  it("should accept custom options", async () => {
    const options = {
      tailLines: 100,
      sinceSeconds: 1800,
    };

    renderHook(() => useArenaJobLogs("staging", "load-test-job", options), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(mockGetArenaJobLogs).toHaveBeenCalled();
    });

    expect(mockGetArenaJobLogs).toHaveBeenCalledWith("staging", "load-test-job", {
      tailLines: 100,
      sinceSeconds: 1800,
    });
  });

  it("should be in loading state initially", () => {
    const { result } = renderHook(() => useArenaJobLogs("production", "eval-job-1"), {
      wrapper: TestWrapper,
    });

    expect(result.current.isLoading).toBe(true);
  });

  it("should handle workspace and job name parameters", async () => {
    const { result } = renderHook(() => useArenaJobLogs("custom-ws", "custom-job"), {
      wrapper: TestWrapper,
    });

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(mockGetArenaJobLogs).toHaveBeenCalledWith(
      "custom-ws",
      "custom-job",
      expect.any(Object)
    );
  });
});
